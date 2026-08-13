# Transferência de Equipe + Mérito/Promoção

## Objetivo

Permitir transferência de membros entre equipes preservando histórico, e registrar méritos/promoções com rastreamento salarial completo para relatórios de investimentos.

## Regras de Negócio

- Membro pode ter no máximo 1 vínculo ativo em qualquer equipe (enforced no backend)
- Transferência = encerrar vínculo antigo + criar vínculo novo (transação atômica)
- Relatórios passados devem refletir composição da equipe naquele período
- Salário novo nunca pode ser menor que salário atual
- Promoções seguem hierarquia de cargos definida

## 1. Modelo de Dados

### 1.1 Alteração em equipe_membros

Adicionar colunas temporais:

```sql
ALTER TABLE equipe_membros
  ADD COLUMN data_entrada TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN data_saida TIMESTAMPTZ;
```

Substituir constraint UNIQUE por partial unique index:

```sql
DROP INDEX equipe_membros_equipe_id_membro_id_key;
CREATE UNIQUE INDEX equipe_membros_active_unique
  ON equipe_membros(equipe_id, membro_id)
  WHERE data_saida IS NULL;
```

Registros existentes: `data_entrada = NOW()` (default), `data_saida = NULL` (vínculo ativo).

### 1.2 Nova tabela historico_salario

```sql
CREATE TABLE historico_salario (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  membro_id UUID NOT NULL REFERENCES membros(id),
  tipo VARCHAR(20) NOT NULL CHECK (tipo IN ('merito', 'promocao')),
  cargo_anterior VARCHAR(100),
  cargo_novo VARCHAR(100),
  salario_anterior NUMERIC(12,2),
  salario_novo NUMERIC(12,2) NOT NULL,
  data_vigencia DATE NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_historico_salario_membro ON historico_salario(membro_id);
```

## 2. Hierarquia de Cargos

Promoções válidas (cargo atual → opções):

| Cargo Atual | Promoções Válidas |
|---|---|
| Analista I | Analista II |
| Analista II | Analista III |
| Analista III | Especialista I, Coordenador de Desenvolvimento |
| Especialista I | Especialista II, Coordenador de Desenvolvimento, Líder Técnico |
| Especialista II | Master, Líder Técnico |
| Coordenador de Desenvolvimento | Líder Técnico |
| Master | (terminal) |
| Líder Técnico | (terminal) |

Hierarquia definida como mapa no backend (domain/cargo.go ou similar). Frontend consome via constante JS espelhada.

## 3. Operações Backend

### 3.1 Transferir Membro

```
POST /equipes/{id}/membros/{membroId}/transferir
Body: { "equipe_destino_id": "uuid" }
Response 200: { "equipe_origem": "nome", "equipe_destino": "nome" }
```

Lógica (em transação):
1. Verificar membro tem vínculo ativo na equipe origem (404 se não)
2. Verificar equipe destino existe (404 se não)
3. `UPDATE equipe_membros SET data_saida = NOW() WHERE membro_id = $1 AND equipe_id = $2 AND data_saida IS NULL`
4. `INSERT INTO equipe_membros (equipe_id, membro_id, data_entrada) VALUES (destino, membro, NOW())`
5. Retornar nomes das equipes

### 3.2 Adicionar Membro (ajuste)

`POST /equipes/{id}/membros` — comportamento existente modificado:
- Antes de inserir, checar vínculo ativo em QUALQUER equipe
- Se existe vínculo ativo em OUTRA equipe: retornar 409 com `{ "conflito": true, "equipe_atual": { "id": "...", "nome": "..." } }`
- Se vínculo ativo na MESMA equipe: retornar 200 (idempotente, já está)
- Se sem vínculo: inserir normalmente com `data_entrada = NOW()`

### 3.3 Remover Membro (ajuste)

`DELETE /equipes/{id}/membros/{membroId}` — em vez de DELETE:
- `UPDATE equipe_membros SET data_saida = NOW() WHERE equipe_id = $1 AND membro_id = $2 AND data_saida IS NULL`
- Preserva registro histórico

### 3.4 Mérito/Promoção

```
POST /membros/{id}/merito-promocao
Body: {
  "tipo": "merito" | "promocao",
  "cargo_novo": "string (obrigatório se tipo=promocao)",
  "salario_novo": 12345.67,
  "data_vigencia": "2026-08-12"
}
Response 200: { "historico_id": "uuid", "antes": {...}, "depois": {...} }
```

Validações:
- `salario_novo >= salario_atual` (400 se menor)
- Se tipo=promocao: `cargo_novo` deve ser promoção válida do cargo atual conforme hierarquia (400 se inválido)
- Se tipo=merito: `cargo_novo` deve ser NULL ou igual ao atual

Lógica:
1. Buscar membro atual (salário, cargo)
2. Validar regras
3. Inserir em `historico_salario` (cargo_anterior, cargo_novo, salario_anterior, salario_novo, data_vigencia)
4. Atualizar `membros`: SET salario = novo, cargo = novo (se promoção), ultimo_aumento = data_vigencia
5. Retornar snapshot antes/depois

## 4. Queries Afetadas

Todas queries que fazem JOIN em equipe_membros precisam filtro temporal:

### 4.1 Membros atuais (listagem, sprint corrente)

```sql
WHERE em.data_saida IS NULL
```

Afeta: `GetMembrosEquipe`, `GetMembrosEquipeIDs`, `GetMembrosEquipeInfo`, `GetAllMembrosEquipe`, `GetUnplannedStats`

### 4.2 Membros em período (relatórios históricos)

```sql
WHERE em.data_entrada <= $fim
  AND (em.data_saida IS NULL OR em.data_saida >= $inicio)
```

Afeta: `GetResumo` (recebe período), `GetCapacity` (recebe dataFim do sprint)

### 4.3 Import

Sem alteração — import usa vínculo ativo (`AddMembroEquipe` já insere com `data_saida IS NULL`).

## 5. Frontend

### 5.1 Auto-Transfer ao Adicionar

Fluxo em `addMembroToEquipe`:
1. POST normal para adicionar
2. Se response 409 com `conflito: true`: mostrar confirm "João já está em Equipe A. Transferir para Equipe B?"
3. Sim → chamar endpoint transferir
4. Não → cancelar

### 5.2 Botão Transferir na Lista de Membros

- Cada membro na lista da equipe ganha botão "↗ Transferir" ao lado de "Remover"
- Click abre dropdown com equipes disponíveis (carregadas via GET /equipes, filtra equipe atual)
- Selecionar destino → confirm → chama endpoint transferir → reload lista

### 5.3 Indicação de data_entrada

- Na lista de membros da equipe, subtexto discreto "desde Ago/2026" (data_entrada formatada)

### 5.4 Modal Mérito/Promoção

Botão "⭐ Mérito/Promoção" na lista de membros da equipe, ao lado de Transferir e Remover.

**Etapa 1 — Formulário:**
- Toggle: Mérito / Promoção
- Se Promoção: dropdown cargo filtrado por hierarquia do cargo atual
- Data (date picker)
- Salário atual (readonly)
- Novo salário (input numérico, validação ≥ atual)
- Percentual de aumento calculado em tempo real: `((novo - atual) / atual * 100).toFixed(1) + '%'`
- Botão "Confirmar"

**Etapa 2 — Antes/Depois:**
- Comparação lado a lado:
  - Cargo: anterior → novo (se promoção)
  - Salário: R$ anterior → R$ novo
  - Aumento: +X%
  - Data vigência
- Botões "Voltar" (volta pra etapa 1) e "Confirmar Definitivo"
- Confirmar → POST backend → fecha modal → reload lista

### 5.5 Dados retornados em GET /equipes/{id}/membros

Adicionar `data_entrada` no response de membros da equipe para exibição no frontend.

## 6. Impacto em Relatórios

### Sprint Capacity
- Filtra membros por período do sprint (data_entrada/data_saida vs data_inicio/data_fim do sprint)
- Membro transferido mid-sprint: aparece na equipe onde estava no fim do sprint

### Resumo Equipe
- Relatório por período mostra membros que estavam na equipe durante aquele período
- Membro transferido aparece em equipe antiga nos relatórios pré-transferência, equipe nova nos pós

### Investimentos
- `historico_salario` alimenta relatório de evolução salarial
- Possível: total investido em promoções/méritos por período/equipe

### Headcount
- Com data_entrada/data_saida, headcount histórico reconstruível para qualquer data

### Dados Existentes
- Migration adiciona colunas com defaults — nenhum dado existente quebra
- Registros atuais em equipe_membros ficam como vínculo ativo (data_saida IS NULL)
