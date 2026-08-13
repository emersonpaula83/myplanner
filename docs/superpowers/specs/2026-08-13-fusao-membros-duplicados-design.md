# Fusão de Membros Duplicados — Design Spec

## Problema

Uma mesma pessoa pode ter mais de uma conta no JIRA (formatos de Atlassian ID
antigo e novo). O sync cria uma linha em `membros` por conta — a chave é
`UNIQUE(fonte_dados_id, jira_account_id)` — então a pessoa aparece duas vezes no
sistema, pode entrar duas vezes na mesma equipe e tem o histórico de tarefas
partido entre os dois registros.

Caso que motivou a spec:

| membro | jira_account_id | equipe | tarefas |
|---|---|---|---|
| `Paulo Cesar W` | `712020:c774549b…` | Devops Varejo | 231 |
| `Paulo Cesar Withoeft` | `6144e81d…` | Devops Varejo | 67 |

O import de planilha (`service/import.go`, `ConfirmImport`) associou a linha da
planilha ao segundo registro e chamou `AddMembroEquipe`, deixando os dois na
equipe. O import se comportou como foi desenhado: associar define para qual
`membro_id` vão os dados da linha, não que dois registros são a mesma pessoa.
Não existe hoje nenhuma forma de fundir membros.

Remover um dos registros da equipe não serve como contorno: esconde as tarefas
daquele registro de todos os relatórios (esforço, capacidade, investimentos).

## Objetivo

Permitir que um operador funda dois registros de membro em um só, de forma que:

- reste exatamente uma linha em `membros` para a pessoa;
- todo o histórico (tarefas, salários, skills, ausências, banco de horas)
  fique no registro sobrevivente;
- a conta JIRA do registro apagado continue reconhecida pelo sync, para que
  atividade nova nela caia no registro sobrevivente em vez de recriar a
  duplicata.

## Fora de escopo

Desfazer fusão, detecção automática de duplicatas, fusão em lote e apelido de
membro.

## Modelo de dados

A identidade JIRA passa a ser 1:N. A conta principal continua em
`membros.jira_account_id`; as contas absorvidas por fusão vão para uma tabela
nova.

```sql
CREATE TABLE membro_jira_contas (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  membro_id       UUID NOT NULL REFERENCES membros(id) ON DELETE CASCADE,
  fonte_dados_id  UUID NOT NULL REFERENCES fonte_dados(id) ON DELETE CASCADE,
  jira_account_id VARCHAR(255) NOT NULL,
  nome_origem     VARCHAR(255),
  criado_em       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (fonte_dados_id, jira_account_id)
);

CREATE INDEX idx_membro_jira_contas_membro ON membro_jira_contas(membro_id);
```

`nome_origem` guarda o nome que a conta tinha no momento da fusão. É o
histórico de nomes da pessoa; o nome do registro sobrevivente não é alterado
pela fusão.

Nenhuma coluna é adicionada a `membros`. Não existe registro "invisível": o
perdedor é apagado, então nenhuma consulta de listagem precisa de filtro novo.

## Operação de fusão

Executada inteiramente dentro de uma transação. Falha em qualquer passo desfaz
tudo.

Nomenclatura: **sobrevivente** é o registro que permanece, **perdedor** é o
registro apagado ao final.

### 1. Repontar referências

As colunas que apontam para `membros`:

| tabela.coluna | regra |
|---|---|
| `tarefas.responsavel_id`, `tarefas.relator_id` | `UPDATE` direto |
| `historico_salario.membro_id` | `UPDATE` direto |
| `membro_salarios.membro_id` | `UPDATE` direto |
| `membro_banco_horas.membro_id` | `UPDATE` direto |
| `disponibilidade.membro_id` | `UPDATE` direto |
| `membro_skills.membro_id` | `UPDATE … ON CONFLICT DO NOTHING`, depois apagar as linhas do perdedor que restaram (`UNIQUE(membro_id, skill_id)`) |
| `membro_produtos.membro_id` | idem, por `UNIQUE(membro_id, produto_id)` |
| `equipe_membros.membro_id` | ver regra abaixo |
| `membros.gestor_id` | `UPDATE` nos membros que tinham o perdedor como gestor; se o sobrevivente ficar como gestor de si mesmo, zerar |
| `projetos.lead_id` | `UPDATE` direto |

Regra de `equipe_membros`: existe `UNIQUE(membro_id) WHERE data_saida IS NULL`,
ou seja, um vínculo ativo por pessoa. Vínculos já encerrados (`data_saida`
preenchida) são repontados normalmente. O vínculo ativo do perdedor só é
repontado se o sobrevivente não tiver um; se tiver, o vínculo do perdedor é
apagado. É esse passo que faz a equipe voltar a mostrar uma pessoa.

Repontar antes de apagar não é opcional: os filhos são `ON DELETE CASCADE` e
`tarefas.responsavel_id`/`relator_id` são `ON DELETE SET NULL`. Apagar o
perdedor sem repontar apagaria históricos e desligaria as tarefas dele.

### 2. Gravar campos de RH escolhidos

O operador escolhe, campo a campo, o valor final de `cargo`, `salario`,
`matricula`, `data_admissao`, `ultimo_aumento` e `gestor_id`. Os valores
escolhidos são gravados no sobrevivente na mesma transação. `nome` não entra na
escolha — fica o do sobrevivente.

### 3. Registrar as contas JIRA

- As linhas de `membro_jira_contas` que pertenciam ao perdedor passam para o
  sobrevivente (cobre o caso de fundir alguém que já absorveu outra conta antes,
  sem regra especial de cadeia).
- A conta do perdedor (`fonte_dados_id`, `jira_account_id`, `nome`) é inserida
  como `membro_jira_contas` do sobrevivente, com `nome_origem` igual ao nome do
  perdedor.

### 4. Apagar o perdedor

`DELETE FROM membros WHERE id = <perdedor>`. Nesse ponto nada mais aponta para
ele.

## Sync

`SyncRepository.UpsertMembro` (`repository/sync.go`) passa a resolver alias
antes do upsert:

1. `SELECT membro_id FROM membro_jira_contas WHERE fonte_dados_id = $1 AND jira_account_id = $2`.
2. Se achou, devolve esse `membro_id` **sem tocar no registro** — o nome do
   sobrevivente não é sobrescrito pelo `displayName` da conta secundária.
3. Se não achou, executa o `INSERT … ON CONFLICT (fonte_dados_id, jira_account_id) DO UPDATE`
   de hoje.

Isso cobre o sync inteiro porque `SyncService.ensureMember`
(`service/sync.go:556`) guarda o retorno em `cache[accountID]` e todo o resto do
sync resolve responsável e relator por esse cache. Tarefa nova na conta
absorvida nasce apontando para o membro sobrevivente.

## API

Dois endpoints, no grupo autenticado, seguindo o padrão de
`/equipes/{id}/membros`:

### `GET /membros/{id}/fusao-preview?alvo={outroId}`

Devolve os dois registros lado a lado para a tela montar a comparação:

- por registro: `id`, `nome`, `email`, `jira_account_id`, `cargo`, `salario`,
  `matricula`, `data_admissao`, `ultimo_aumento`, `gestor_id`, `gestor_nome`;
- por registro: contagem de tarefas (`responsavel_id`), equipe do vínculo ativo,
  contagem de registros de histórico salarial, skills e ausências;
- sugestão de sobrevivente: o registro com mais tarefas.

Erros: 400 se `id == alvo` ou se algum dos dois não existe.

### `POST /membros/{id}/fundir`

`{id}` é o sobrevivente. Corpo:

```json
{
  "membro_perdedor_id": "uuid",
  "campos": {
    "cargo": "analista_iii",
    "salario": 12345.67,
    "matricula": "0001",
    "data_admissao": "2020-01-31",
    "ultimo_aumento": "2025-06-01",
    "gestor_id": "uuid|null"
  }
}
```

Cada campo em `campos` é o valor final desejado; ausência do campo significa
"não alterar o que o sobrevivente já tem". Resposta: contagem do que foi
repontado (tarefas, vínculos de equipe, registros de histórico) para a tela
confirmar o resultado.

Erros: 400 para IDs iguais, membro inexistente. 500 para falha na transação, com
a fusão desfeita.

Não há endpoint de desfazer. Reverter exigiria recriar o registro apagado e
saber quais linhas voltar — outro projeto.

## Interface

Entrada pelo card de membro na tela de Equipes (`frontend/index.html`, render em
`.member-row`), um terceiro botão `btn-member-action` ao lado de ↗ Transferir e
⭐ Mérito, com `event.stopPropagation()` porque a linha inteira abre o detalhe do
membro.

**Passo 1 — escolher o duplicado.** Campo de busca no mesmo padrão do
"Pesquisar membro para adicionar" já existente (`searchMembroToAdd`,
`GET /membros?search=`), com o membro clicado fixo como um dos lados. Busca
manual, sem sugestão automática: um teste no banco mostrou 18 pares de nomes
parecidos, dos quais apenas 2 eram duplicatas reais.

**Passo 2 — comparar.** Modal em duas colunas. Cabeçalho por registro com nome,
e-mail, `jira_account_id`, contagem de tarefas e equipe atual. Uma linha por
campo de RH:

- valor preenchido só de um lado: exibido já selecionado, sem pedir decisão;
- valores diferentes nos dois lados: dois botões de rádio, nenhum pré-marcado;
  o botão de confirmar fica bloqueado enquanto houver conflito sem resposta;
- campo vazio dos dois lados: não aparece.

Sobrevivente sugerido é o registro com mais tarefas, com um toggle para
inverter. O nome que fica é o do sobrevivente.

**Confirmação.** Botão destrutivo com o efeito em números concretos, por
exemplo: "231 + 67 tarefas passam para Paulo Cesar W · 1 vínculo de equipe
duplicado será encerrado · esta ação não tem desfazer". Após confirmar,
recarrega o resumo da equipe.

## Testes

**Unidade** (`service/membro_merge_test.go`, com o padrão de mock de
`service/mocks_test.go`):

- campo em conflito resolve pela escolha do operador;
- campo preenchido só de um lado é adotado sem escolha;
- fundir um membro consigo mesmo é recusado;
- fundir membro inexistente é recusado;
- sugestão de sobrevivente segue a contagem de tarefas.

**Integração contra o Postgres de dev**, marcada com skip quando não há banco
(padrão de `repository/usuario_saml_test.go`), porque o valor da funcionalidade
está no SQL:

- tarefas do perdedor passam para o sobrevivente e nenhuma fica com
  `responsavel_id` nulo;
- vínculo ativo duplicado é encerrado sem violar `equipe_membros_active_unique`;
- `membro_skills` e `membro_produtos` ficam sem duplicata;
- membro perdedor deixa de existir e some de `GET /membros`;
- `UpsertMembro` com a conta absorvida devolve o ID do sobrevivente e não altera
  o nome dele;
- fundir um membro que já absorveu outra conta move as duas contas.

## Riscos

- **Fusão errada é cara.** Não há desfazer e o registro é apagado. Mitigação: a
  tela mostra contagens antes de confirmar e exige decisão explícita em cada
  campo conflitante.
- **Conta absorvida reaparecendo.** Se `membro_jira_contas` for perdida (por
  exemplo, recriação de `fonte_dados` com CASCADE), o sync volta a criar a
  duplicata. É o mesmo risco que qualquer dado de configuração da fonte.
