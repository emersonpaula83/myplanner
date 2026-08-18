package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
)

// FonteDadosStore abstracts *repository.FonteDadosRepository for testing.
// Consumed by: SyncService, AllocationService, EqualizerService, SprintGenerationService.
type FonteDadosStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error)
	SaveOAuthTokens(ctx context.Context, id uuid.UUID, baseURL, accessToken, refreshToken string, expiry time.Time) error
	UpdateUltimoSync(ctx context.Context, id uuid.UUID, syncTime time.Time) error
}

// SprintRepoStore abstracts *repository.SprintRepository for testing.
// Named "RepoStore" to avoid collision with existing SprintStore (handler→service interface).
// Consumed by: SprintService, AllocationService, NotificationService, SprintGenerationService.
// Note: EqualizerService uses concrete *repository.SprintRepository (needs Pool() for raw query).
type SprintRepoStore interface {
	ListProjetosComSprints(ctx context.Context, equipeID *uuid.UUID) ([]repository.ProjetoComSprints, error)
	ListByProjeto(ctx context.Context, projetoID uuid.UUID, estado *string) ([]repository.SprintListItem, error)
	GetEquipeBoardID(ctx context.Context, equipeID uuid.UUID) (*int, error)
	ListSprints(ctx context.Context, equipeID *uuid.UUID, estado *string, boardID *int) ([]repository.SprintListItem, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Sprint, error)
	GetProjetoChave(ctx context.Context, projetoID uuid.UUID) (string, error)
	GetFeriadosNoPeriodo(ctx context.Context, inicio, fim time.Time) ([]repository.FeriadoRecord, error)
	GetMembrosEquipeIDs(ctx context.Context, equipeID uuid.UUID, dataFim time.Time) (map[uuid.UUID]bool, error)
	GetMembrosEquipeInfo(ctx context.Context, equipeID uuid.UUID, dataFim time.Time) ([]repository.MembroInfo, error)
	GetMembrosFromSprint(ctx context.Context, sprintID uuid.UUID) ([]repository.MembroInfo, error)
	GetTarefasDetailBySprint(ctx context.Context, sprintID uuid.UUID) ([]repository.TarefaDetail, error)
	GetAusenciasNoPeriodo(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) ([]repository.AusenciaRecord, error)
	GetUnplannedStats(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) (*repository.UnplannedStats, error)
	GetEquipeNome(ctx context.Context, equipeID uuid.UUID) (string, error)
	GetSprintProjetoID(ctx context.Context, sprintID uuid.UUID) (*uuid.UUID, error)
	GetHistoricalUnplanned(ctx context.Context, projetoID uuid.UUID, equipeID *uuid.UUID, currentSprintID uuid.UUID, limit int) ([]repository.HistoricalUnplannedItem, error)
	GetDisclaimerTasks(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID, taskType string) ([]repository.DisclaimerTarefaRow, error)
	GetDisclaimerTarefaProdutos(ctx context.Context, tarefaIDs []uuid.UUID) (map[uuid.UUID][]string, error)
	GetBurndownTarefas(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) ([]repository.BurndownTarefa, error)
	ListSprintsIncludeEmpty(ctx context.Context, equipeID *uuid.UUID, estado *string, boardID *int) ([]repository.SprintListItem, error)
	GetAllMembrosEquipe(ctx context.Context, equipeID uuid.UUID) ([]repository.MembroInfo, error)
	GetHorasAlocadasPorSprint(ctx context.Context, sprintIDs []uuid.UUID, membroIDs []uuid.UUID) (map[uuid.UUID]float64, error)
	GetTimelineDetailTarefas(ctx context.Context, sprintID uuid.UUID, equipeID uuid.UUID) ([]repository.TimelineDetailTarefa, error)
	GetMembroJiraAccountID(ctx context.Context, membroID uuid.UUID) (string, error)
	GetEqualizerTarefas(ctx context.Context, sprintID, membroID uuid.UUID) ([]repository.EqualizerTarefa, error)
	UpdateTarefaResponsavel(ctx context.Context, sprintID, tarefaID, novoResponsavelID uuid.UUID) error
}

// SyncRepoStore abstracts *repository.SyncRepository for testing.
// Named "RepoStore" to avoid collision with existing SyncStore (handler→service interface).
// Consumed by: SyncService, SprintGenerationService.
type SyncRepoStore interface {
	HasRunningSyncForProject(ctx context.Context, fonteDadosID uuid.UUID, projectKey string) (bool, error)
	CreateSyncLog(ctx context.Context, log *domain.SyncLog) error
	UpdateSyncLog(ctx context.Context, id uuid.UUID, status string, finalizadoEm time.Time, totals repository.SyncTotals, erros json.RawMessage, mensagem *string) error
	GetFonteDadosAtivas(ctx context.Context) ([]domain.FonteDados, error)
	GetLatestSyncLog(ctx context.Context, fonteDadosID uuid.UUID) (*domain.SyncLog, error)
	GetAggregatedSyncStatus(ctx context.Context, fonteDadosID uuid.UUID) (*domain.SyncLog, error)
	ListSyncLogs(ctx context.Context, fonteDadosID uuid.UUID, limit int) ([]domain.SyncLog, error)
	HasRunningSync(ctx context.Context, fonteDadosID uuid.UUID) (bool, error)
	UpdateSyncLogTotals(ctx context.Context, id uuid.UUID, totals repository.SyncTotals) error
	LookupTarefaIDByJiraID(ctx context.Context, fonteDadosID uuid.UUID, jiraID string) (uuid.UUID, error)
	UpdateTarefaParent(ctx context.Context, tarefaID, parentID uuid.UUID) error
	UpdateCustomFieldMap(ctx context.Context, fonteID uuid.UUID, cfMap json.RawMessage) error
	GetProjectKeysForSync(ctx context.Context, fonteDadosID uuid.UUID) ([]string, error)
	UpsertProduto(ctx context.Context, fonteDadosID uuid.UUID, jiraID, nome string, descricao *string, projetoID *uuid.UUID) (uuid.UUID, error)
	LinkTarefaProduto(ctx context.Context, tarefaID, produtoID uuid.UUID) error
	UndeleteReappearedTarefas(ctx context.Context, fonteDadosID uuid.UUID, presentJiraIDs []string) (int64, error)
	SoftDeleteAbsentTarefas(ctx context.Context, fonteDadosID uuid.UUID, presentJiraIDs []string) (int64, error)
	UpsertProjeto(ctx context.Context, fonteDadosID uuid.UUID, jiraID, chave, nome string, descricao *string, leadID *uuid.UUID, categoria *string) (uuid.UUID, error)
	UpsertMembro(ctx context.Context, fonteDadosID uuid.UUID, jiraAccountID, nome string, email, avatarURL, team *string) (uuid.UUID, error)
	GetDistinctBoardProjects(ctx context.Context, fonteDadosID uuid.UUID) (map[int]uuid.UUID, error)
	UpsertSprint(ctx context.Context, fonteDadosID uuid.UUID, jiraID int, nome string, estado *string, dataInicio, dataFim, dataConclusao *time.Time, boardID *int, projetoID *uuid.UUID) (uuid.UUID, error)
	UpsertTarefa(ctx context.Context, t *repository.UpsertTarefaParams) (uuid.UUID, error)
	AutoDetectEquipeBoardIDs(ctx context.Context, fonteDadosID uuid.UUID) (int, error)
}

// AllocationStore abstracts *repository.AllocationRepository for testing.
// Consumed by: AllocationService.
type AllocationStore interface {
	GetEpicsByEquipeAndProduto(ctx context.Context, equipeID uuid.UUID, produtoNomes []string, statusFilter string) ([]repository.EpicAllocationRow, error)
	CheckGDPTCAncestors(ctx context.Context, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error)
	GetClosedEpicIDs(ctx context.Context, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error)
	GetProjectClosure(ctx context.Context, epicID uuid.UUID) (*repository.ProjectClosureRow, error)
	GetEpicByID(ctx context.Context, epicID uuid.UUID) (*repository.EpicAllocationRow, error)
	GetEpicTasks(ctx context.Context, epicID uuid.UUID) ([]repository.TaskAllocationRow, error)
	GetEpicPeople(ctx context.Context, epicID uuid.UUID) ([]repository.PersonAllocationRow, error)
	GetTaskPreviousState(ctx context.Context, taskID uuid.UUID) (*repository.TaskPreviousState, error)
	UpdateTaskAllocation(ctx context.Context, taskID, sprintID uuid.UUID, assigneeID *uuid.UUID, estimateSeconds int) error
	GetTaskJiraKey(ctx context.Context, taskID uuid.UUID) (string, error)
	GetTaskFonteDadosID(ctx context.Context, taskID uuid.UUID) (uuid.UUID, error)
	GetSprintJiraID(ctx context.Context, sprintID uuid.UUID) (int, error)
	RollbackTaskAllocation(ctx context.Context, taskID uuid.UUID, prev *repository.TaskPreviousState) error
	GetFutureSprintsByEquipe(ctx context.Context, equipeID uuid.UUID) ([]repository.SprintOptionRow, error)
	CloseProject(ctx context.Context, epicID uuid.UUID, descricao string, dataEncerramento time.Time, encerradoPor string) error
	ReopenProject(ctx context.Context, epicID uuid.UUID) error
	GetProdutosComProjetosAtivos(ctx context.Context) ([]repository.ProdutoRow, error)
}

// ReviewStore abstracts *repository.ReviewRepository for testing.
// Consumed by: ReviewService.
type ReviewStore interface {
	GetReviewAnalise(ctx context.Context, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*repository.ReviewAnalise, error)
	GetSprintEstado(ctx context.Context, sprintID uuid.UUID) (*string, error)
	GetSprintSnapshot(ctx context.Context, sprintID uuid.UUID) ([]repository.ReviewTaskRow, error)
	GetReviewTasks(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID, produtoIDs []uuid.UUID) ([]repository.ReviewTaskRow, error)
	GetGDPTCAncestorTaskIDs(ctx context.Context, taskIDs []uuid.UUID) ([]uuid.UUID, error)
	GetReviewPOs(ctx context.Context, equipeID uuid.UUID, produtoIDs []uuid.UUID) ([]repository.ReviewPO, error)
	ListDestaques(ctx context.Context, sprintID, equipeID uuid.UUID) ([]repository.ReviewDestaque, error)
	CreateDestaque(ctx context.Context, d repository.ReviewDestaque) (repository.ReviewDestaque, error)
	UpdateDestaque(ctx context.Context, id uuid.UUID, titulo, descricao string, link *string) (repository.ReviewDestaque, error)
	DeleteDestaque(ctx context.Context, id uuid.UUID) error
	SaveReviewAnalise(ctx context.Context, a repository.ReviewAnalise) error
}

// ConfigStore abstracts *repository.ConfigRepository for testing.
// Consumed by: ReviewService, EmailProvider, WhatsAppProvider, EqualizerService.
type ConfigStore interface {
	GetConfig(ctx context.Context, chave string) (string, error)
}

// EquipeStore abstracts *repository.EquipeRepository for testing.
// Consumed by: SprintGenerationService.
// Note: ImportService/InvestimentoService also use EquipeRepository but are out of SP1 scope.
type EquipeStore interface {
	GetMembrosEquipe(ctx context.Context, equipeID uuid.UUID) ([]domain.Membro, error)
}

// DestinatarioStore abstracts *repository.DestinatarioRepository for testing.
// Consumed by: NotificationService.
type DestinatarioStore interface {
	GetByIDs(ctx context.Context, ids []uuid.UUID) ([]repository.Destinatario, error)
}

// SyncScheduleStore abstracts *repository.SyncScheduleRepository for testing.
// Consumed by: SchedulerService.
type SyncScheduleStore interface {
	GetDueSchedules(ctx context.Context, horaMinuto string) ([]domain.SyncSchedule, error)
}

// PlanningRepoStore abstracts *repository.PlanningRepository for testing.
// Consumed by: PlanningService.
type PlanningRepoStore interface {
	GetNextSprint(ctx context.Context, boardID int, currentDataInicio time.Time) (*domain.Sprint, error)
	GetAllTarefasBySprint(ctx context.Context, sprintID uuid.UUID) ([]repository.PlanningTarefa, error)
	UpdateTarefaEstimativa(ctx context.Context, tarefaID uuid.UUID, segundos int) error
	UpdateTarefaTipoDemanda(ctx context.Context, tarefaID uuid.UUID, valor string) error
	UpdateTarefaResponsavel(ctx context.Context, tarefaID uuid.UUID, responsavelID *uuid.UUID) error
	MoveTarefaToSprint(ctx context.Context, tarefaID uuid.UUID, sprintID uuid.UUID) error
	RemoveTarefaFromSprint(ctx context.Context, tarefaID uuid.UUID) error
	GetSprintJiraID(ctx context.Context, sprintID uuid.UUID) (int, error)
	SearchTarefasByKeys(ctx context.Context, projetoID uuid.UUID, keys []string) ([]repository.SearchTarefaResult, error)
	UpsertTarefaFromJira(ctx context.Context, t *repository.UpsertTarefaParams) (uuid.UUID, error)
	MoveTarefasToSprint(ctx context.Context, sprintID uuid.UUID, tarefaIDs []uuid.UUID) error
	GetTarefasByIDs(ctx context.Context, ids []uuid.UUID) ([]repository.PlanningTarefa, error)
	GetProjetoChaveByID(ctx context.Context, projetoID uuid.UUID) (string, uuid.UUID, error)
}
