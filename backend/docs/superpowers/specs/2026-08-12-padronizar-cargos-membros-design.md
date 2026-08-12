# Padronizar Cargos de Membros — Design Spec

## Objetivo

Substituir a lista atual de cargos de membros por cargos que refletem a estrutura real da organização. Migrar dados existentes.

## Contexto

`membros.cargo` é um campo free-text validado contra `domain.CargosValidos`. Os valores atuais (desenvolvedor, po_produto, scrum_master, etc.) não correspondem à hierarquia real. A organização usa: Analista I/II/III, Especialista I/II, Master, Coordenador de Desenvolvimento e Líder Técnico.

## Escopo

### Incluído

- Atualizar `domain/cargo.go` com novos cargos
- Migration SQL para migrar dados existentes
- Atualizar frontend (labels, badges, dropdown)

### Excluído

- `handler/usuario.go` `cargosValidos` — são roles de acesso do sistema (coordenador, gerente, gerente_projetos), não cargos organizacionais. Não toca.

## Novos Cargos

| Slug | Label | Badge Color |
|------|-------|-------------|
| `analista_i` | Analista I | gray |
| `analista_ii` | Analista II | gray |
| `analista_iii` | Analista III | blue |
| `especialista_i` | Especialista I | blue |
| `especialista_ii` | Especialista II | purple |
| `master` | Master | purple |
| `coordenador_desenvolvimento` | Coord. Dev | green |
| `lider_tecnico` | Líder Técnico | green |

## Migration

```sql
-- Migrar desenvolvedor → analista_iii
UPDATE membros SET cargo = 'analista_iii' WHERE cargo = 'desenvolvedor';

-- Cargos removidos → NULL (reassignar manualmente via dropdown)
UPDATE membros SET cargo = NULL WHERE cargo IN (
  'po_produto', 'gerente_tecnologia', 'gerente_executivo',
  'scrum_master', 'agile_master'
);
```

Down migration restaura valores originais — não é possível reverter com precisão, então apenas re-adiciona os slugs ao código sem alterar dados.

## Arquivos Afetados

### Backend

- `backend/internal/domain/cargo.go` — substituir constantes, `CargosValidos`, `CargoLabels`
- `backend/migrations/000029_padronizar_cargos.up.sql` — UPDATE statements
- `backend/migrations/000029_padronizar_cargos.down.sql` — reverter constantes no código (dados não revertidos)

### Frontend

- `frontend/index.html`:
  - `cargoLabelMap()` (~line 2375) — novos labels
  - `cargoBadgeColor()` (~line 2388) — novas cores
  - Dropdown items no detalhe do membro (~line 6370) — nova lista

### Testes

- `backend/internal/handler/equipe.go:373` — validação `IsCargoValido` já funciona contra a lista; testes existentes que usam `"desenvolvedor"` precisam atualizar pra `"analista_iii"`
- Verificar testes em `equipe_test.go` e `investimento_test.go` que referenciam cargos

## Restrições

- Não criar tabela separada de cargos — manter como enum no código (padrão existente)
- `coordenador_desenvolvimento` mantém o mesmo slug (já existe no DB e no código)
- Migration deve ser idempotente (WHERE clause garante)
