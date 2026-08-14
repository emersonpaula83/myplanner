# Cortina de Salários — Design Spec

## Problema

Valores salariais aparecem hoje para qualquer usuário autenticado: salário por
membro, investimento mensal da equipe, gasto mensal acumulado, histórico
salarial e o preview do import de planilha. Basta abrir a tela de Investimentos.

Esconder no frontend não resolve: os números chegam no JSON da API, então
qualquer pessoa com o F12 aberto lê tudo na aba Network, mesmo com a tela
mascarada.

## Objetivo

Valores de dinheiro ficam mascarados por padrão. Um cadeado ao lado do valor
abre um modal que pede a senha do usuário logado; com a senha correta e cargo
autorizado, os valores passam a ser exibidos até o usuário sair do sistema.

Requisito duro: **enquanto travado, o número não sai do servidor**. Não existe
valor escondido em atributo HTML, em `display:none` ou em variável de JavaScript.

## Decisões tomadas

| Decisão | Escolha |
|---|---|
| Senha exigida | Do próprio usuário logado, não uma senha compartilhada |
| Quem pode destravar | Cargos `coordenador`, `gerente`, `diretor` e a conta admin |
| Duração | Até sair do sistema (vida do token, 24h) |
| Telas cobertas | Investimentos, detalhe do membro, modal Mérito/Promoção, preview do import |
| Mecanismo | Claim no JWT, sem estado novo no servidor |

O cargo `diretor` não existe hoje e é criado por esta feature.

## Mecanismo

### Claim no token

`auth.Claims` (`backend/internal/auth/jwt.go:11`) ganha:

```go
Salarios bool `json:"salarios,omitempty"`
```

`GenerateToken` passa a receber o flag. O login sempre emite `false`.

### `POST /auth/desbloquear-salarios`

Corpo: `{"senha": "..."}`. Passos:

1. Lê `email` e `cargo` do contexto — o middleware de auth já injeta ambos
   (`middleware/auth.go:68`).
2. **403** se o cargo não for `coordenador`, `gerente` ou `diretor`. A conta
   admin (`ADMIN_EMAIL`) passa independente do cargo.
3. Busca o usuário por e-mail e compara a senha com
   `bcrypt.CompareHashAndPassword`, como `AuthHandler.Login`
   (`handler/auth.go:80`). **401** se não bater.
4. Emite token novo com o mesmo `sub`, `email` e `cargo`, agora com
   `salarios: true`, preservando **a expiração restante do token atual**.
   Renovar a expiração transformaria o desbloqueio num jeito de nunca deslogar.
5. Responde `{"token": "..."}`. O frontend substitui o token no `localStorage`.

### `POST /auth/travar-salarios`

Devolve um token igual, sem a claim. Não pede senha. É o que faz o cadeado
voltar a fechar sem logout.

### Cargo `diretor`

Entra em `cargosValidos` (`handler/usuario.go:18`) e no select de cargo da tela
de usuários. Sem migration: `usuarios.cargo` é `VARCHAR(50)` sem CHECK.

## Backend: onde os valores são segurados

Uma função `middleware.PodeVerSalarios(ctx) bool` lê a claim do contexto, no
mesmo padrão de `GetUserCargo`. Cada handler afetado a consulta e limpa a
resposta antes do `respondJSON`. A limpeza é explícita — nada de mágica na
serialização.

### Leitura

| Rota | Campo limpo quando travado |
|---|---|
| `GET /membros`, `/membros/{id}`, `/membros/search` | `salario` |
| `GET /equipes/{id}/membros` | `salario` de cada membro |
| `GET /equipes/{id}/investimentos` | `sumario.custo_mensal_total` e `salario` de cada membro |
| `GET /equipes/{id}/investimentos/gastos-mensais` | `meses[].custo_total` |
| `GET /membros/{id}/salario/historico` | lista vazia |
| `POST /investimentos/import`, `/import/sync` | `salario` de cada linha e o marcador `salario` em `changes` |

`domain.Membro.Salario` e `MembroInvestimento.Salario` já são `*float64` com
`omitempty`: virar `nil` faz a chave sumir do JSON.

Campos que hoje são `float64` puro — `InvestimentoSumario.CustoMensalTotal`,
`GastoMensal.CustoTotal` e `SalarioHistorico.Valor` — passam a `*float64` com
`omitempty`. Zerar seria mentir: "R$ 0,00" é um valor, e o frontend não
distinguiria de custo real zero. Ausente é ausente.

### Escrita

Recusadas com **403** sem a claim:

- `PUT /membros/{id}/salario`
- `POST /membros/{id}/merito-promocao`
- `POST /investimentos/import/confirmar`

Travar só a leitura seria teatro: sem isso, quem abre o F12 monta o `PUT` na mão
e altera salário sem nunca ter destravado.

### Fora da cortina

`banco_horas`, `tempo_casa`, `cargo` e contagem de membros continuam visíveis.
São da mesma tela, mas não são dinheiro, e travá-los esvaziaria a página.

## Interface

O frontend já decodifica o payload do JWT (`index.html:2173`). Uma função
`salariosDesbloqueados()` lê a claim `salarios` do token guardado — a fonte da
verdade é o mesmo token que o backend confere, sem estado paralelo.

**Valor travado** renderiza `••••• 🔒`, com o cadeado clicável. Uma única função
`valorSalario(v)` decide entre formatar o número e devolver a máscara, e
substitui as chamadas diretas a `formatSalarioBR` nos pontos cobertos — para não
existirem dois jeitos de esconder.

**Modal de senha:** campo `type="password"`, foco automático, Enter envia. Erros
inline, sem `alert`: 401 vira "Senha incorreta", 403 vira "Seu cargo não permite
ver valores salariais". No sucesso, troca o token no `localStorage`, fecha o
modal e chama a função de carga da tela atual (`loadInvestimentos`,
`loadMembroDetail` ou o preview do import). Os valores aparecem porque vieram do
servidor, não porque estavam escondidos.

**Botão no cabeçalho** das telas com dinheiro: 🔒 quando travado (abre o modal),
🔓 quando destravado (chama `/auth/travar-salarios`, troca o token, recarrega).

**Telas de escrita:**

- O botão ⭐ Mérito/Promoção, travado, abre o modal de senha em vez do modal de
  mérito: a tela depende do salário atual para calcular o percentual.
- No preview do import, a coluna Salário mostra a máscara e o botão **Confirmar
  Importação** fica desabilitado com a frase "Destrave os valores para revisar os
  salários antes de importar". Sem isso, alguém confirmaria às cegas uma planilha
  que altera salário, e o backend responderia 403 sem explicação.

## Testes

**Unidade — token** (`auth/jwt_test.go`):

- token gerado com `salarios: true` sobrevive ao round-trip de validação;
- token gerado sem a claim valida como `false` — o `omitempty` não pode virar
  `true` por acidente.

**Unidade — handlers** (`handler/salario_lock_test.go`, mock store no padrão de
`handler/import_test.go`):

- cargo não autorizado devolve 403 sem consultar a senha;
- senha errada devolve 401;
- senha certa devolve token com a claim e com a **mesma expiração** do token de
  entrada;
- `travar-salarios` devolve token sem a claim.

**Por rota de leitura afetada:** sem a claim, o campo não existe no corpo. A
asserção é sobre o JSON cru (`w.Body.String()` não contém `custo_mensal_total`),
não sobre o struct — é o que prova o requisito do F12.

**Por rota de escrita:** sem a claim, 403, e o store não é chamado.

## Fora de escopo

- Auditoria de quem destravou e quando
- Expiração por tempo (15 min) — descartada em favor de "até sair do sistema"
- Mascarar `banco_horas`
- Restringir por equipe: quem destrava vê o custo de todas as equipes a que já
  tem acesso hoje

## Risco aceito

O desbloqueio vive no token por até 24 horas. Máquina desbloqueada e deixada
aberta mostra salário até o logout. É a consequência direta da duração
escolhida; a alternativa avaliada e descartada era retravar após 15 minutos.
