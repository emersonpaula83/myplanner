# Import Planilha Investimentos — Design Spec

## Objetivo

Permitir importar dados financeiros de membros a partir de planilha (CSV upload ou URL Google Sheets pública), com resolução manual de nomes/equipes não encontrados e botão Sync para atualizações recorrentes.

## Contexto

Dados financeiros (salário, admissão, cargo, matrícula) vivem numa planilha Google Sheets mantida pelo RH. Hoje são inseridos manualmente membro a membro. O import automatiza isso com match por nome e resolução manual de pendências.

## Formato da Planilha

CSV com colunas (header na linha 5 do arquivo original, mas CSV exportado pode ter header na linha 1):

| Coluna | Tipo | Exemplo |
|--------|------|---------|
| Nome | string | RICARDO KAZUO DINIZ NOZAKI |
| Gestão | string | Angela Kanegae Oda |
| Time / Squad | string | DEVOPS RM |
| Função | string | ANALISTA II DE DESENVOLVIMENTO CLOUD |
| Matrícula | string | 000101016701 |
| Admissão | date (dd/mm/yyyy) | 18/05/2026 |
| Salário | currency BR | R$ 6.480,00 |
| Último Aumento | date (dd/mm/yyyy) ou serial Excel | 01/01/2026 |

### Regras de parsing

- Ignora linhas com "SUB" no campo Nome
- Ignora linha de total (última linha com contagem numérica no campo Nome)
- Ignora linhas vazias e headers
- Salário: remove "R$ ", substitui "." por "", substitui "," por "." → parseFloat
- Datas dd/mm/yyyy: converte para YYYY-MM-DD
- Datas como número serial Excel (ex: 46083): converte para date (epoch Excel = 1899-12-30 + serial days)
- Matrícula "-" ou vazia: trata como NULL
- Último Aumento "-" ou vazio: trata como NULL
- Membros sem matrícula MAS com dados válidos: importar normalmente (não são SUBs)

## Fluxo

### Fase 1: Upload/Fetch

1. Usuário clica "Importar" na tela Investimentos
2. Modal abre com duas abas: **Upload CSV** | **Google Sheets URL**
3. Upload CSV: file picker aceita .csv
4. Google Sheets URL: campo de texto. Backend extrai spreadsheet ID e gid da URL, faz GET em `https://docs.google.com/spreadsheets/d/{ID}/gviz/tq?tqx=out:csv&gid={GID}` (requer planilha pública: "qualquer pessoa com o link pode ver")
5. Backend parseia CSV, faz match e retorna resultado

### Fase 2: Resolução

Backend retorna:

```json
{
  "matched": [
    {
      "linha": 1,
      "nome_planilha": "RICARDO KAZUO DINIZ NOZAKI",
      "membro_id": "uuid",
      "membro_nome": "Ricardo Kazuo Diniz Nozaki",
      "equipe_id": "uuid",
      "equipe_nome": "Devops RM",
      "dados": {
        "cargo": "ANALISTA II DE DESENVOLVIMENTO CLOUD",
        "matricula": "000101016701",
        "salario": 6480.00,
        "data_admissao": "2026-05-18",
        "ultimo_aumento": "2026-05-18",
        "gestor_nome": "Angela Kanegae Oda",
        "gestor_id": "uuid"
      },
      "changes": ["salario", "data_admissao"]
    }
  ],
  "unmatched_membros": [
    {
      "linha": 5,
      "nome_planilha": "FULANO DE TAL",
      "dados": { ... }
    }
  ],
  "unmatched_equipes": [
    {
      "nome_planilha": "DEVOPS NOVA",
      "linhas": [5, 8]
    }
  ],
  "unmatched_gestores": [
    {
      "nome_planilha": "Novo Gestor",
      "linhas": [5]
    }
  ],
  "ignorados": [
    { "linha": 3, "nome": "SUB 167064 - AGILISTA", "motivo": "SUB" }
  ]
}
```

Frontend mostra modal de resolução:

**A) Membros não encontrados:**
- Nome da planilha + dropdown com todos membros do sistema
- Opção "Ignorar"

**B) Equipes não encontradas:**
- Nome da planilha + dropdown com equipes existentes
- Opção "Criar equipe" (cria com nome informado)

**C) Gestores não encontrados:**
- Nome da planilha + dropdown com membros do sistema
- Opção "Ignorar"

**D) Preview:**
- Tabela com todos membros matched + resolvidos
- Colunas: Nome, Equipe, Cargo, Salário, Admissão, Matrícula, Ação (update/sem mudança)
- Botão "Confirmar Importação"

### Fase 3: Confirmação

`POST /investimentos/import/confirmar` recebe os mapeamentos resolvidos. Backend aplica:

- Atualiza `membros.salario` (só valor atual, sem histórico)
- Atualiza `membros.cargo` (mapeia Função da planilha → slug do sistema, match parcial)
- Atualiza `membros.data_admissao`
- Atualiza `membros.matricula` (campo novo)
- Atualiza `membros.ultimo_aumento` (campo novo)
- Atualiza `membros.gestor_id` (campo novo)
- Se membro tem `data_admissao` futura: frontend mostra badge "novo" sobre avatar

### Fase 4: Sync

Após primeira importação, salva config. Botão "Sync" na tela:
- Se tipo=sheets_url: busca CSV automaticamente e repete fluxo
- Se tipo=csv: pede novo upload e repete fluxo
- Mostra data do último sync

## Modelo de Dados

### Novos campos em `membros`

```sql
ALTER TABLE membros ADD COLUMN matricula VARCHAR(20);
ALTER TABLE membros ADD COLUMN ultimo_aumento DATE;
ALTER TABLE membros ADD COLUMN gestor_id UUID REFERENCES membros(id);
```

### Nova tabela `import_configs`

```sql
CREATE TABLE import_configs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tipo VARCHAR(20) NOT NULL, -- 'csv' ou 'sheets_url'
  url TEXT,
  gid VARCHAR(20),
  ultimo_sync TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Apenas uma config ativa por vez (UPSERT com ON CONFLICT ou delete + insert).

## Endpoints

| Method | Path | Descrição |
|--------|------|-----------|
| POST | `/investimentos/import` | Recebe CSV (multipart file) ou JSON com sheets_url. Retorna resultado de match |
| POST | `/investimentos/import/confirmar` | Recebe mapeamentos resolvidos, aplica updates |
| GET | `/investimentos/import/config` | Retorna config de sync salva (tipo, url, ultimo_sync) |
| POST | `/investimentos/import/sync` | Re-executa import com config salva |

## Mapeamento Cargo Planilha → Sistema

A planilha traz cargo completo (ex: "ANALISTA II DE DESENVOLVIMENTO CLOUD"). Backend extrai o nível:

| Contém | Cargo sistema |
|--------|---------------|
| "ANALISTA I " (com espaço) ou termina em "ANALISTA I" | analista_i |
| "ANALISTA II " ou termina em "ANALISTA II" | analista_ii |
| "ANALISTA III" | analista_iii |
| "ESPECIALISTA I " ou termina em "ESPECIALISTA I" | especialista_i |
| "ESPECIALISTA II" | especialista_ii |
| "MASTER" | master |
| "COORDENADOR" | coordenador_desenvolvimento |
| "LÍDER" ou "LIDER" | lider_tecnico |
| "TÉCNICO" ou "TECNICO" | analista_i |
| Nenhum match | NULL (não altera cargo atual) |

## Frontend

### Botão Importar

Na tela Investimentos, ao lado do filtro de equipe. Ícone de upload + "Importar".

### Botão Sync

Aparece após primeira importação bem-sucedida. Mostra data do último sync. Ícone de refresh.

### Badge "Novo"

Membros com `data_admissao` futura (> hoje): badge "NOVO" sobre avatar na linha de avatares e na tabela. Badge verde com texto branco.

## Restrições

- Planilha Google Sheets precisa estar pública ("qualquer pessoa com o link") pra sync funcionar
- Se planilha privada e URL informada: backend retorna erro claro pedindo pra tornar pública
- CSV upload sempre funciona independente de permissões
- Import não cria membros novos — só atualiza existentes (membros vêm do Jira sync)
- Import não deleta membros — só atualiza campos financeiros
- Uma config de sync por vez (sobrescreve anterior)
