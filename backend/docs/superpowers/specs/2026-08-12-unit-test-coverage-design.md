# Unit Test Coverage Baseline (Régua de Testes Unitários)

> **Meta:** 70% de cobertura unitária no backend Go, partindo de ~18% atual.

**Abordagem:** Stdlib puro — `testing` nativo, mocks manuais, zero dependências externas (sem testify/mockery).

**Escopo:** Backend Go (`internal/`) apenas. Repository layer excluído de testes (camada SQL pura). Todos testes que envolvem BD usam mocks.

---

## Global Constraints

- Go stdlib only — `testing` package, no external test frameworks
- Hand-written mock structs (function-field pattern already used in codebase)
- Interfaces defined at consumer (service package), not provider (repository package)
- Repository layer (4,775 lines) excluded from unit test coverage — tested via integration tests if needed
- No changes to repository package
- `coverage.out` and `coverage.html` added to `.gitignore`

---

## 1. Arquitetura de Testabilidade

### Problema

Toda camada service depende de `*repository.XxxRepository` concreto. Sem interfaces, impossível mockar repositórios em testes unitários.

### Solução

Interfaces definidas no pacote consumer (`service`), seguindo Go idiomático ("accept interfaces, return structs").

**Arquivos novos:**

| Arquivo | Propósito |
|---------|-----------|
| `internal/service/interfaces.go` | Todas interfaces de repository (9 interfaces) |
| `internal/service/mocks_test.go` | Todos mock structs para testes |

### Interfaces a Criar

| Interface | Repository Real | Services que Consomem |
|-----------|----------------|----------------------|
| `FonteDadosStore` | `*FonteDadosRepository` | AllocationService, SprintGenerationService, SyncService, EqualizerService |
| `SprintStore` | `*SprintRepository` | SprintService, AllocationService, NotificationService, EqualizerService, SprintGenerationService |
| `SyncStore` | `*SyncRepository` | SyncService, SprintGenerationService |
| `AllocationStore` | `*AllocationRepository` | AllocationService |
| `ReviewStore` | `*ReviewRepository` | ReviewService |
| `ConfigStore` | `*ConfigRepository` | ReviewService, EmailProvider, WhatsAppProvider, EqualizerService |
| `EquipeStore` | `*EquipeRepository` | SprintGenerationService |
| `DestinatarioStore` | `*DestinatarioRepository` | NotificationService |
| `SyncScheduleStore` | `*SyncScheduleRepository` | SchedulerService |

**Nota:** `SprintStore` e `SyncStore` já existem parcialmente no codebase. Expandir com métodos faltantes.

### Padrão de Interface

Cada interface expõe SOMENTE métodos que o service realmente chama:

```go
// interfaces.go
package service

import (
    "context"
    "time"

    "github.com/emersonpaula83/myplanner/backend/internal/repository"
    "github.com/google/uuid"
)

type FonteDadosStore interface {
    GetByID(ctx context.Context, id uuid.UUID) (*repository.FonteDados, error)
    SaveOAuthTokens(ctx context.Context, id uuid.UUID, baseURL, access, refresh string, expiry time.Time) error
    UpdateUltimoSync(ctx context.Context, id uuid.UUID, ts time.Time) error
}
```

### Padrão de Mock

Function-field pattern (consistente com `mockJiraClient` existente):

```go
// mocks_test.go
package service

type mockFonteDadosStore struct {
    getByIDFn       func(ctx context.Context, id uuid.UUID) (*repository.FonteDados, error)
    saveOAuthFn     func(ctx context.Context, id uuid.UUID, baseURL, access, refresh string, expiry time.Time) error
    updateUltimoFn  func(ctx context.Context, id uuid.UUID, ts time.Time) error
}

func (m *mockFonteDadosStore) GetByID(ctx context.Context, id uuid.UUID) (*repository.FonteDados, error) {
    return m.getByIDFn(ctx, id)
}

func (m *mockFonteDadosStore) SaveOAuthTokens(ctx context.Context, id uuid.UUID, baseURL, access, refresh string, expiry time.Time) error {
    return m.saveOAuthFn(ctx, id, baseURL, access, refresh, expiry)
}

func (m *mockFonteDadosStore) UpdateUltimoSync(ctx context.Context, id uuid.UUID, ts time.Time) error {
    return m.updateUltimoFn(ctx, id, ts)
}
```

### Refatoração dos Service Structs

```go
// ANTES:
type SyncService struct {
    repo   *repository.SyncRepository
    fdRepo *repository.FonteDadosRepository
}

// DEPOIS:
type SyncService struct {
    repo   SyncStore
    fdRepo FonteDadosStore
}
```

Constructors (`NewXxxService`) mudam signature para aceitar interfaces. `*repository.XxxRepository` implementa a interface automaticamente — zero mudança em `main.go`.

---

## 2. Metas de Cobertura por Pacote

| Pacote | Linhas | Cobertura Atual | Meta | Linhas a Cobrir |
|--------|:------:|:---:|:---:|:---:|
| `service` | 5,113 | 4.3% | 70% | ~3,360 |
| `handler` | 4,151 | 20.5% | 70% | ~2,055 |
| `jira` | 962 | 30.9% | 60% | ~280 |
| `admin` | 263 | 0% | 60% | ~158 |
| `middleware` | 277 | 77.6% | 80% | ~7 |
| `auth` | 237 | 86.9% | 87%+ | ~0 |
| `repository` | 4,775 | 0% | excluído | — |
| **Total** | **~16,800** | **~18%** | **70%** | **~5,860** |

---

## 3. Prioridade de Implementação

### Service Layer (P1 — maior impacto)

| Ordem | Service | Linhas aprox. | Justificativa |
|:-----:|---------|:---:|---------------|
| 1 | SyncService | ~1,800 | Maior service, sync JIRA, muitos edge cases |
| 2 | SprintService | ~1,200 | Dashboard/relatórios, cálculos complexos |
| 3 | AllocationService | ~800 | Lógica de alocação com rollback |
| 4 | ReviewService | ~500 | Análise sprint review |
| 5 | SprintGenerationService | ~400 | Já tem 14 testes, expandir |
| 6 | EqualizerService | ~300 | Redistribuição tarefas |
| 7 | Notification/Email/WhatsApp | ~200 | Menores, mais simples |
| 8 | SchedulerService | ~50 | 1 método |

### Handler Layer (P2)

1. Criar interfaces para 7 handlers com dependências concretas
2. Criar mocks para novas interfaces
3. Expandir testes dos 13 handlers que já usam interfaces

### Pacotes Menores (P3)

- `jira`: expandir cobertura de parsing/client
- `admin`: cobertura básica

---

## 4. Enforcement

### Makefile Targets

```makefile
.PHONY: test test-cover test-cover-html

test:
	go test ./internal/...

test-cover:
	go test ./internal/... -coverprofile=coverage.out
	@go tool cover -func=coverage.out | grep total | awk '{print $$3}'
	@echo "---"
	@go tool cover -func=coverage.out | grep -E "^github" | awk '{split($$1,a,"/"); pkg=a[length(a)-1]"/"a[length(a)]; print pkg, $$NF}' | sort -t'.' -k1 -rn

test-cover-html: test-cover
	go tool cover -html=coverage.out -o coverage.html
	@echo "Open coverage.html in browser"
```

### Script de Threshold (`scripts/check-coverage.sh`)

```bash
#!/bin/bash
MIN_TOTAL=70.0
EXCLUDE="repository"

TOTAL=$(go test ./internal/... -coverprofile=coverage.out 2>/dev/null \
  | grep -v "/$EXCLUDE" \
  | grep "coverage:" \
  | awk '{sum+=$2; n++} END {printf "%.1f", sum/n}')

echo "Coverage: ${TOTAL}%"
if (( $(echo "$TOTAL < $MIN_TOTAL" | bc -l) )); then
  echo "FAIL: coverage ${TOTAL}% < minimum ${MIN_TOTAL}%"
  exit 1
fi
echo "PASS"
```

### Integração

- Dev local: `make test-cover` para verificar cobertura
- CI (futuro): `scripts/check-coverage.sh` no pipeline
- `coverage.out` e `coverage.html` no `.gitignore`

---

## 5. Decomposição em Sub-projetos

Projeto grande demais para implementação única. Cada sub-projeto gera spec → plan → implementação própria.

| Sub-projeto | Escopo | Estimativa | Dependências |
|-------------|--------|:---:|:---:|
| **SP1: Infraestrutura** | Extrair 9 interfaces, criar mock structs, refatorar service structs, Makefile targets | ~2-3h | Nenhuma |
| **SP2: Service — SyncService** | Testes unitários SyncService | ~4-5h | SP1 |
| **SP3: Service — Sprint + Allocation** | Testes SprintService + AllocationService | ~3-4h | SP1 |
| **SP4: Service — Restantes** | Review, Equalizer, SprintGen, Notification, Scheduler | ~3-4h | SP1 |
| **SP5: Handler tests** | Interfaces pros 7 handlers concretos + testes novos | ~3-4h | SP1 (parcial) |
| **SP6: Pacotes menores + enforcement** | jira, admin + script threshold + CI integration | ~2h | Nenhuma |

**Ordem:** SP1 primeiro (habilita todos os outros), depois SP2-SP4 em qualquer ordem, SP5 depende parcialmente de SP1, SP6 por último.

```
SP1 (infraestrutura)
 ├── SP2 (SyncService)
 ├── SP3 (Sprint + Allocation)
 ├── SP4 (restantes services)
 └── SP5 (handlers)
SP6 (menores + enforcement) — independente
```

---

## 6. Decisões Registradas

| Decisão | Escolha | Alternativa descartada |
|---------|---------|----------------------|
| Framework de teste | stdlib puro (`testing`) | testify, gomock, mockery |
| Localização de interfaces | Consumer (service pkg) | Provider (repository pkg) |
| Geração de mocks | Manual (function-field) | Code generation (mockgen) |
| Repository coverage | Excluído (0% aceito) | pgxmock para testar SQL |
| Organização de interfaces | Arquivo centralizado | 1 arquivo por interface |
| Meta de cobertura | 70% overall | Per-package enforcement |
