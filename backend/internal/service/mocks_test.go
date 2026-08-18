package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
)

// --- mockFonteDadosStore ---

type mockFonteDadosStore struct {
	getByIDFn          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error)
	saveOAuthTokensFn  func(ctx context.Context, id uuid.UUID, baseURL, accessToken, refreshToken string, expiry time.Time) error
	updateUltimoSyncFn func(ctx context.Context, id uuid.UUID, syncTime time.Time) error
}

func (m *mockFonteDadosStore) GetByID(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) {
	return m.getByIDFn(ctx, id)
}
func (m *mockFonteDadosStore) SaveOAuthTokens(ctx context.Context, id uuid.UUID, baseURL, accessToken, refreshToken string, expiry time.Time) error {
	return m.saveOAuthTokensFn(ctx, id, baseURL, accessToken, refreshToken, expiry)
}
func (m *mockFonteDadosStore) UpdateUltimoSync(ctx context.Context, id uuid.UUID, syncTime time.Time) error {
	return m.updateUltimoSyncFn(ctx, id, syncTime)
}

// --- mockSprintRepoStore ---

type mockSprintRepoStore struct {
	listProjetosComSprintsFn    func(ctx context.Context, equipeID *uuid.UUID) ([]repository.ProjetoComSprints, error)
	listByProjetoFn             func(ctx context.Context, projetoID uuid.UUID, estado *string) ([]repository.SprintListItem, error)
	getEquipeBoardIDFn          func(ctx context.Context, equipeID uuid.UUID) (*int, error)
	listSprintsFn               func(ctx context.Context, equipeID *uuid.UUID, estado *string, boardID *int) ([]repository.SprintListItem, error)
	getByIDFn                   func(ctx context.Context, id uuid.UUID) (*domain.Sprint, error)
	getProjetoChaveFn           func(ctx context.Context, projetoID uuid.UUID) (string, error)
	getFeriadosNoPeriodoFn      func(ctx context.Context, inicio, fim time.Time) ([]repository.FeriadoRecord, error)
	getMembrosEquipeIDsFn       func(ctx context.Context, equipeID uuid.UUID, dataFim time.Time) (map[uuid.UUID]bool, error)
	getMembrosEquipeInfoFn      func(ctx context.Context, equipeID uuid.UUID, dataFim time.Time) ([]repository.MembroInfo, error)
	getMembrosFromSprintFn      func(ctx context.Context, sprintID uuid.UUID) ([]repository.MembroInfo, error)
	getTarefasDetailBySprintFn  func(ctx context.Context, sprintID uuid.UUID) ([]repository.TarefaDetail, error)
	getAusenciasNoPeriodoFn     func(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) ([]repository.AusenciaRecord, error)
	getUnplannedStatsFn         func(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) (*repository.UnplannedStats, error)
	getEquipeNomeFn             func(ctx context.Context, equipeID uuid.UUID) (string, error)
	getSprintProjetoIDFn        func(ctx context.Context, sprintID uuid.UUID) (*uuid.UUID, error)
	getHistoricalUnplannedFn    func(ctx context.Context, projetoID uuid.UUID, equipeID *uuid.UUID, currentSprintID uuid.UUID, limit int) ([]repository.HistoricalUnplannedItem, error)
	getDisclaimerTasksFn        func(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID, taskType string) ([]repository.DisclaimerTarefaRow, error)
	getDisclaimerTarefaProdFn   func(ctx context.Context, tarefaIDs []uuid.UUID) (map[uuid.UUID][]string, error)
	getBurndownTarefasFn        func(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) ([]repository.BurndownTarefa, error)
	listSprintsIncludeEmptyFn   func(ctx context.Context, equipeID *uuid.UUID, estado *string, boardID *int) ([]repository.SprintListItem, error)
	getAllMembrosEquipeFn        func(ctx context.Context, equipeID uuid.UUID) ([]repository.MembroInfo, error)
	getHorasAlocadasPorSprintFn func(ctx context.Context, sprintIDs []uuid.UUID, membroIDs []uuid.UUID) (map[uuid.UUID]float64, error)
	getTimelineDetailTarefasFn  func(ctx context.Context, sprintID uuid.UUID, equipeID uuid.UUID) ([]repository.TimelineDetailTarefa, error)
	getMembroJiraAccountIDFn    func(ctx context.Context, membroID uuid.UUID) (string, error)
	getEqualizerTarefasFn       func(ctx context.Context, sprintID, membroID uuid.UUID) ([]repository.EqualizerTarefa, error)
	updateTarefaResponsavelFn   func(ctx context.Context, sprintID, tarefaID, novoResponsavelID uuid.UUID) error
}

func (m *mockSprintRepoStore) ListProjetosComSprints(ctx context.Context, equipeID *uuid.UUID) ([]repository.ProjetoComSprints, error) {
	return m.listProjetosComSprintsFn(ctx, equipeID)
}
func (m *mockSprintRepoStore) ListByProjeto(ctx context.Context, projetoID uuid.UUID, estado *string) ([]repository.SprintListItem, error) {
	return m.listByProjetoFn(ctx, projetoID, estado)
}
func (m *mockSprintRepoStore) GetEquipeBoardID(ctx context.Context, equipeID uuid.UUID) (*int, error) {
	return m.getEquipeBoardIDFn(ctx, equipeID)
}
func (m *mockSprintRepoStore) ListSprints(ctx context.Context, equipeID *uuid.UUID, estado *string, boardID *int) ([]repository.SprintListItem, error) {
	return m.listSprintsFn(ctx, equipeID, estado, boardID)
}
func (m *mockSprintRepoStore) GetByID(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
	return m.getByIDFn(ctx, id)
}
func (m *mockSprintRepoStore) GetProjetoChave(ctx context.Context, projetoID uuid.UUID) (string, error) {
	return m.getProjetoChaveFn(ctx, projetoID)
}
func (m *mockSprintRepoStore) GetFeriadosNoPeriodo(ctx context.Context, inicio, fim time.Time) ([]repository.FeriadoRecord, error) {
	return m.getFeriadosNoPeriodoFn(ctx, inicio, fim)
}
func (m *mockSprintRepoStore) GetMembrosEquipeIDs(ctx context.Context, equipeID uuid.UUID, dataFim time.Time) (map[uuid.UUID]bool, error) {
	return m.getMembrosEquipeIDsFn(ctx, equipeID, dataFim)
}
func (m *mockSprintRepoStore) GetMembrosEquipeInfo(ctx context.Context, equipeID uuid.UUID, dataFim time.Time) ([]repository.MembroInfo, error) {
	return m.getMembrosEquipeInfoFn(ctx, equipeID, dataFim)
}
func (m *mockSprintRepoStore) GetMembrosFromSprint(ctx context.Context, sprintID uuid.UUID) ([]repository.MembroInfo, error) {
	return m.getMembrosFromSprintFn(ctx, sprintID)
}
func (m *mockSprintRepoStore) GetTarefasDetailBySprint(ctx context.Context, sprintID uuid.UUID) ([]repository.TarefaDetail, error) {
	return m.getTarefasDetailBySprintFn(ctx, sprintID)
}
func (m *mockSprintRepoStore) GetAusenciasNoPeriodo(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) ([]repository.AusenciaRecord, error) {
	return m.getAusenciasNoPeriodoFn(ctx, membroIDs, inicio, fim)
}
func (m *mockSprintRepoStore) GetUnplannedStats(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) (*repository.UnplannedStats, error) {
	return m.getUnplannedStatsFn(ctx, sprintID, equipeID)
}
func (m *mockSprintRepoStore) GetEquipeNome(ctx context.Context, equipeID uuid.UUID) (string, error) {
	return m.getEquipeNomeFn(ctx, equipeID)
}
func (m *mockSprintRepoStore) GetSprintProjetoID(ctx context.Context, sprintID uuid.UUID) (*uuid.UUID, error) {
	return m.getSprintProjetoIDFn(ctx, sprintID)
}
func (m *mockSprintRepoStore) GetHistoricalUnplanned(ctx context.Context, projetoID uuid.UUID, equipeID *uuid.UUID, currentSprintID uuid.UUID, limit int) ([]repository.HistoricalUnplannedItem, error) {
	return m.getHistoricalUnplannedFn(ctx, projetoID, equipeID, currentSprintID, limit)
}
func (m *mockSprintRepoStore) GetDisclaimerTasks(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID, taskType string) ([]repository.DisclaimerTarefaRow, error) {
	return m.getDisclaimerTasksFn(ctx, sprintID, equipeID, taskType)
}
func (m *mockSprintRepoStore) GetDisclaimerTarefaProdutos(ctx context.Context, tarefaIDs []uuid.UUID) (map[uuid.UUID][]string, error) {
	return m.getDisclaimerTarefaProdFn(ctx, tarefaIDs)
}
func (m *mockSprintRepoStore) GetBurndownTarefas(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) ([]repository.BurndownTarefa, error) {
	return m.getBurndownTarefasFn(ctx, sprintID, equipeID)
}
func (m *mockSprintRepoStore) ListSprintsIncludeEmpty(ctx context.Context, equipeID *uuid.UUID, estado *string, boardID *int) ([]repository.SprintListItem, error) {
	return m.listSprintsIncludeEmptyFn(ctx, equipeID, estado, boardID)
}
func (m *mockSprintRepoStore) GetAllMembrosEquipe(ctx context.Context, equipeID uuid.UUID) ([]repository.MembroInfo, error) {
	return m.getAllMembrosEquipeFn(ctx, equipeID)
}
func (m *mockSprintRepoStore) GetHorasAlocadasPorSprint(ctx context.Context, sprintIDs []uuid.UUID, membroIDs []uuid.UUID) (map[uuid.UUID]float64, error) {
	return m.getHorasAlocadasPorSprintFn(ctx, sprintIDs, membroIDs)
}
func (m *mockSprintRepoStore) GetTimelineDetailTarefas(ctx context.Context, sprintID uuid.UUID, equipeID uuid.UUID) ([]repository.TimelineDetailTarefa, error) {
	return m.getTimelineDetailTarefasFn(ctx, sprintID, equipeID)
}
func (m *mockSprintRepoStore) GetMembroJiraAccountID(ctx context.Context, membroID uuid.UUID) (string, error) {
	return m.getMembroJiraAccountIDFn(ctx, membroID)
}
func (m *mockSprintRepoStore) GetEqualizerTarefas(ctx context.Context, sprintID, membroID uuid.UUID) ([]repository.EqualizerTarefa, error) {
	return m.getEqualizerTarefasFn(ctx, sprintID, membroID)
}
func (m *mockSprintRepoStore) UpdateTarefaResponsavel(ctx context.Context, sprintID, tarefaID, novoResponsavelID uuid.UUID) error {
	return m.updateTarefaResponsavelFn(ctx, sprintID, tarefaID, novoResponsavelID)
}

// --- mockSyncRepoStore ---

type mockSyncRepoStore struct {
	hasRunningSyncForProjectFn func(ctx context.Context, fonteDadosID uuid.UUID, projectKey string) (bool, error)
	createSyncLogFn            func(ctx context.Context, log *domain.SyncLog) error
	updateSyncLogFn            func(ctx context.Context, id uuid.UUID, status string, finalizadoEm time.Time, totals repository.SyncTotals, erros json.RawMessage, mensagem *string) error
	getFonteDadosAtivasFn      func(ctx context.Context) ([]domain.FonteDados, error)
	getLatestSyncLogFn         func(ctx context.Context, fonteDadosID uuid.UUID) (*domain.SyncLog, error)
	getAggregatedSyncStatusFn  func(ctx context.Context, fonteDadosID uuid.UUID) (*domain.SyncLog, error)
	listSyncLogsFn             func(ctx context.Context, fonteDadosID uuid.UUID, limit int) ([]domain.SyncLog, error)
	hasRunningSyncFn            func(ctx context.Context, fonteDadosID uuid.UUID) (bool, error)
	updateSyncLogTotalsFn      func(ctx context.Context, id uuid.UUID, totals repository.SyncTotals) error
	lookupTarefaIDByJiraIDFn   func(ctx context.Context, fonteDadosID uuid.UUID, jiraID string) (uuid.UUID, error)
	updateTarefaParentFn       func(ctx context.Context, tarefaID, parentID uuid.UUID) error
	updateCustomFieldMapFn     func(ctx context.Context, fonteID uuid.UUID, cfMap json.RawMessage) error
	getProjectKeysForSyncFn    func(ctx context.Context, fonteDadosID uuid.UUID) ([]string, error)
	upsertProdutoFn            func(ctx context.Context, fonteDadosID uuid.UUID, jiraID, nome string, descricao *string, projetoID *uuid.UUID) (uuid.UUID, error)
	linkTarefaProdutoFn        func(ctx context.Context, tarefaID, produtoID uuid.UUID) error
	undeleteReappearedFn       func(ctx context.Context, fonteDadosID uuid.UUID, presentJiraIDs []string) (int64, error)
	softDeleteAbsentFn         func(ctx context.Context, fonteDadosID uuid.UUID, presentJiraIDs []string) (int64, error)
	upsertProjetoFn            func(ctx context.Context, fonteDadosID uuid.UUID, jiraID, chave, nome string, descricao *string, leadID *uuid.UUID, categoria *string) (uuid.UUID, error)
	upsertMembroFn             func(ctx context.Context, fonteDadosID uuid.UUID, jiraAccountID, nome string, email, avatarURL, team *string) (uuid.UUID, error)
	getDistinctBoardProjectsFn func(ctx context.Context, fonteDadosID uuid.UUID) (map[int]uuid.UUID, error)
	upsertSprintFn             func(ctx context.Context, fonteDadosID uuid.UUID, jiraID int, nome string, estado *string, dataInicio, dataFim, dataConclusao *time.Time, boardID *int, projetoID *uuid.UUID) (uuid.UUID, error)
	upsertTarefaFn             func(ctx context.Context, t *repository.UpsertTarefaParams) (uuid.UUID, error)
	autoDetectEquipeBoardIDsFn func(ctx context.Context, fonteDadosID uuid.UUID) (int, error)
}

func (m *mockSyncRepoStore) HasRunningSyncForProject(ctx context.Context, fonteDadosID uuid.UUID, projectKey string) (bool, error) {
	return m.hasRunningSyncForProjectFn(ctx, fonteDadosID, projectKey)
}
func (m *mockSyncRepoStore) CreateSyncLog(ctx context.Context, log *domain.SyncLog) error {
	return m.createSyncLogFn(ctx, log)
}
func (m *mockSyncRepoStore) UpdateSyncLog(ctx context.Context, id uuid.UUID, status string, finalizadoEm time.Time, totals repository.SyncTotals, erros json.RawMessage, mensagem *string) error {
	return m.updateSyncLogFn(ctx, id, status, finalizadoEm, totals, erros, mensagem)
}
func (m *mockSyncRepoStore) GetFonteDadosAtivas(ctx context.Context) ([]domain.FonteDados, error) {
	return m.getFonteDadosAtivasFn(ctx)
}
func (m *mockSyncRepoStore) GetLatestSyncLog(ctx context.Context, fonteDadosID uuid.UUID) (*domain.SyncLog, error) {
	return m.getLatestSyncLogFn(ctx, fonteDadosID)
}
func (m *mockSyncRepoStore) GetAggregatedSyncStatus(ctx context.Context, fonteDadosID uuid.UUID) (*domain.SyncLog, error) {
	if m.getAggregatedSyncStatusFn != nil {
		return m.getAggregatedSyncStatusFn(ctx, fonteDadosID)
	}
	return m.getLatestSyncLogFn(ctx, fonteDadosID)
}
func (m *mockSyncRepoStore) ListSyncLogs(ctx context.Context, fonteDadosID uuid.UUID, limit int) ([]domain.SyncLog, error) {
	return m.listSyncLogsFn(ctx, fonteDadosID, limit)
}
func (m *mockSyncRepoStore) HasRunningSync(ctx context.Context, fonteDadosID uuid.UUID) (bool, error) {
	return m.hasRunningSyncFn(ctx, fonteDadosID)
}
func (m *mockSyncRepoStore) UpdateSyncLogTotals(ctx context.Context, id uuid.UUID, totals repository.SyncTotals) error {
	return m.updateSyncLogTotalsFn(ctx, id, totals)
}
func (m *mockSyncRepoStore) LookupTarefaIDByJiraID(ctx context.Context, fonteDadosID uuid.UUID, jiraID string) (uuid.UUID, error) {
	return m.lookupTarefaIDByJiraIDFn(ctx, fonteDadosID, jiraID)
}
func (m *mockSyncRepoStore) UpdateTarefaParent(ctx context.Context, tarefaID, parentID uuid.UUID) error {
	return m.updateTarefaParentFn(ctx, tarefaID, parentID)
}
func (m *mockSyncRepoStore) UpdateCustomFieldMap(ctx context.Context, fonteID uuid.UUID, cfMap json.RawMessage) error {
	return m.updateCustomFieldMapFn(ctx, fonteID, cfMap)
}
func (m *mockSyncRepoStore) GetProjectKeysForSync(ctx context.Context, fonteDadosID uuid.UUID) ([]string, error) {
	return m.getProjectKeysForSyncFn(ctx, fonteDadosID)
}
func (m *mockSyncRepoStore) UpsertProduto(ctx context.Context, fonteDadosID uuid.UUID, jiraID, nome string, descricao *string, projetoID *uuid.UUID) (uuid.UUID, error) {
	return m.upsertProdutoFn(ctx, fonteDadosID, jiraID, nome, descricao, projetoID)
}
func (m *mockSyncRepoStore) LinkTarefaProduto(ctx context.Context, tarefaID, produtoID uuid.UUID) error {
	return m.linkTarefaProdutoFn(ctx, tarefaID, produtoID)
}
func (m *mockSyncRepoStore) UndeleteReappearedTarefas(ctx context.Context, fonteDadosID uuid.UUID, presentJiraIDs []string) (int64, error) {
	return m.undeleteReappearedFn(ctx, fonteDadosID, presentJiraIDs)
}
func (m *mockSyncRepoStore) SoftDeleteAbsentTarefas(ctx context.Context, fonteDadosID uuid.UUID, presentJiraIDs []string) (int64, error) {
	return m.softDeleteAbsentFn(ctx, fonteDadosID, presentJiraIDs)
}
func (m *mockSyncRepoStore) UpsertProjeto(ctx context.Context, fonteDadosID uuid.UUID, jiraID, chave, nome string, descricao *string, leadID *uuid.UUID, categoria *string) (uuid.UUID, error) {
	return m.upsertProjetoFn(ctx, fonteDadosID, jiraID, chave, nome, descricao, leadID, categoria)
}
func (m *mockSyncRepoStore) UpsertMembro(ctx context.Context, fonteDadosID uuid.UUID, jiraAccountID, nome string, email, avatarURL, team *string) (uuid.UUID, error) {
	return m.upsertMembroFn(ctx, fonteDadosID, jiraAccountID, nome, email, avatarURL, team)
}
func (m *mockSyncRepoStore) GetDistinctBoardProjects(ctx context.Context, fonteDadosID uuid.UUID) (map[int]uuid.UUID, error) {
	return m.getDistinctBoardProjectsFn(ctx, fonteDadosID)
}
func (m *mockSyncRepoStore) UpsertSprint(ctx context.Context, fonteDadosID uuid.UUID, jiraID int, nome string, estado *string, dataInicio, dataFim, dataConclusao *time.Time, boardID *int, projetoID *uuid.UUID) (uuid.UUID, error) {
	return m.upsertSprintFn(ctx, fonteDadosID, jiraID, nome, estado, dataInicio, dataFim, dataConclusao, boardID, projetoID)
}
func (m *mockSyncRepoStore) UpsertTarefa(ctx context.Context, t *repository.UpsertTarefaParams) (uuid.UUID, error) {
	return m.upsertTarefaFn(ctx, t)
}
func (m *mockSyncRepoStore) AutoDetectEquipeBoardIDs(ctx context.Context, fonteDadosID uuid.UUID) (int, error) {
	return m.autoDetectEquipeBoardIDsFn(ctx, fonteDadosID)
}

// --- mockAllocationStore ---

type mockAllocationStore struct {
	getEpicsByEquipeAndProdutoFn func(ctx context.Context, equipeID uuid.UUID, produtoNomes []string, statusFilter string) ([]repository.EpicAllocationRow, error)
	checkGDPTCAncestorsFn       func(ctx context.Context, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error)
	getClosedEpicIDsFn          func(ctx context.Context, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error)
	getProjectClosureFn         func(ctx context.Context, epicID uuid.UUID) (*repository.ProjectClosureRow, error)
	getEpicByIDFn               func(ctx context.Context, epicID uuid.UUID) (*repository.EpicAllocationRow, error)
	getEpicTasksFn              func(ctx context.Context, epicID uuid.UUID) ([]repository.TaskAllocationRow, error)
	getEpicPeopleFn             func(ctx context.Context, epicID uuid.UUID) ([]repository.PersonAllocationRow, error)
	getTaskPreviousStateFn      func(ctx context.Context, taskID uuid.UUID) (*repository.TaskPreviousState, error)
	updateTaskAllocationFn      func(ctx context.Context, taskID, sprintID uuid.UUID, assigneeID *uuid.UUID, estimateSeconds int) error
	getTaskJiraKeyFn            func(ctx context.Context, taskID uuid.UUID) (string, error)
	getTaskFonteDadosIDFn       func(ctx context.Context, taskID uuid.UUID) (uuid.UUID, error)
	getSprintJiraIDFn           func(ctx context.Context, sprintID uuid.UUID) (int, error)
	rollbackTaskAllocationFn    func(ctx context.Context, taskID uuid.UUID, prev *repository.TaskPreviousState) error
	getFutureSprintsByEquipeFn  func(ctx context.Context, equipeID uuid.UUID) ([]repository.SprintOptionRow, error)
	closeProjectFn              func(ctx context.Context, epicID uuid.UUID, descricao string, dataEncerramento time.Time, encerradoPor string) error
	reopenProjectFn             func(ctx context.Context, epicID uuid.UUID) error
	getProdutosComProjetosAtvFn func(ctx context.Context) ([]repository.ProdutoRow, error)
}

func (m *mockAllocationStore) GetEpicsByEquipeAndProduto(ctx context.Context, equipeID uuid.UUID, produtoNomes []string, statusFilter string) ([]repository.EpicAllocationRow, error) {
	return m.getEpicsByEquipeAndProdutoFn(ctx, equipeID, produtoNomes, statusFilter)
}
func (m *mockAllocationStore) CheckGDPTCAncestors(ctx context.Context, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	return m.checkGDPTCAncestorsFn(ctx, epicIDs)
}
func (m *mockAllocationStore) GetClosedEpicIDs(ctx context.Context, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	return m.getClosedEpicIDsFn(ctx, epicIDs)
}
func (m *mockAllocationStore) GetProjectClosure(ctx context.Context, epicID uuid.UUID) (*repository.ProjectClosureRow, error) {
	return m.getProjectClosureFn(ctx, epicID)
}
func (m *mockAllocationStore) GetEpicByID(ctx context.Context, epicID uuid.UUID) (*repository.EpicAllocationRow, error) {
	return m.getEpicByIDFn(ctx, epicID)
}
func (m *mockAllocationStore) GetEpicTasks(ctx context.Context, epicID uuid.UUID) ([]repository.TaskAllocationRow, error) {
	return m.getEpicTasksFn(ctx, epicID)
}
func (m *mockAllocationStore) GetEpicPeople(ctx context.Context, epicID uuid.UUID) ([]repository.PersonAllocationRow, error) {
	return m.getEpicPeopleFn(ctx, epicID)
}
func (m *mockAllocationStore) GetTaskPreviousState(ctx context.Context, taskID uuid.UUID) (*repository.TaskPreviousState, error) {
	return m.getTaskPreviousStateFn(ctx, taskID)
}
func (m *mockAllocationStore) UpdateTaskAllocation(ctx context.Context, taskID, sprintID uuid.UUID, assigneeID *uuid.UUID, estimateSeconds int) error {
	return m.updateTaskAllocationFn(ctx, taskID, sprintID, assigneeID, estimateSeconds)
}
func (m *mockAllocationStore) GetTaskJiraKey(ctx context.Context, taskID uuid.UUID) (string, error) {
	return m.getTaskJiraKeyFn(ctx, taskID)
}
func (m *mockAllocationStore) GetTaskFonteDadosID(ctx context.Context, taskID uuid.UUID) (uuid.UUID, error) {
	return m.getTaskFonteDadosIDFn(ctx, taskID)
}
func (m *mockAllocationStore) GetSprintJiraID(ctx context.Context, sprintID uuid.UUID) (int, error) {
	return m.getSprintJiraIDFn(ctx, sprintID)
}
func (m *mockAllocationStore) RollbackTaskAllocation(ctx context.Context, taskID uuid.UUID, prev *repository.TaskPreviousState) error {
	return m.rollbackTaskAllocationFn(ctx, taskID, prev)
}
func (m *mockAllocationStore) GetFutureSprintsByEquipe(ctx context.Context, equipeID uuid.UUID) ([]repository.SprintOptionRow, error) {
	return m.getFutureSprintsByEquipeFn(ctx, equipeID)
}
func (m *mockAllocationStore) CloseProject(ctx context.Context, epicID uuid.UUID, descricao string, dataEncerramento time.Time, encerradoPor string) error {
	return m.closeProjectFn(ctx, epicID, descricao, dataEncerramento, encerradoPor)
}
func (m *mockAllocationStore) ReopenProject(ctx context.Context, epicID uuid.UUID) error {
	return m.reopenProjectFn(ctx, epicID)
}
func (m *mockAllocationStore) GetProdutosComProjetosAtivos(ctx context.Context) ([]repository.ProdutoRow, error) {
	return m.getProdutosComProjetosAtvFn(ctx)
}

// --- mockReviewStore ---

type mockReviewStore struct {
	getReviewAnaliseFn      func(ctx context.Context, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*repository.ReviewAnalise, error)
	getSprintEstadoFn       func(ctx context.Context, sprintID uuid.UUID) (*string, error)
	getSprintSnapshotFn     func(ctx context.Context, sprintID uuid.UUID) ([]repository.ReviewTaskRow, error)
	getReviewTasksFn        func(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID, produtoIDs []uuid.UUID) ([]repository.ReviewTaskRow, error)
	getGDPTCAncestorIDsFn   func(ctx context.Context, taskIDs []uuid.UUID) ([]uuid.UUID, error)
	getReviewPOsFn          func(ctx context.Context, equipeID uuid.UUID, produtoIDs []uuid.UUID) ([]repository.ReviewPO, error)
	listDestaquesFn         func(ctx context.Context, sprintID, equipeID uuid.UUID) ([]repository.ReviewDestaque, error)
	createDestaqueFn        func(ctx context.Context, d repository.ReviewDestaque) (repository.ReviewDestaque, error)
	updateDestaqueFn        func(ctx context.Context, id uuid.UUID, titulo, descricao string, link *string) (repository.ReviewDestaque, error)
	deleteDestaqueFn        func(ctx context.Context, id uuid.UUID) error
	saveReviewAnaliseFn     func(ctx context.Context, a repository.ReviewAnalise) error
}

func (m *mockReviewStore) GetReviewAnalise(ctx context.Context, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*repository.ReviewAnalise, error) {
	return m.getReviewAnaliseFn(ctx, sprintID, equipeID, produtoIDs)
}
func (m *mockReviewStore) GetSprintEstado(ctx context.Context, sprintID uuid.UUID) (*string, error) {
	return m.getSprintEstadoFn(ctx, sprintID)
}
func (m *mockReviewStore) GetSprintSnapshot(ctx context.Context, sprintID uuid.UUID) ([]repository.ReviewTaskRow, error) {
	return m.getSprintSnapshotFn(ctx, sprintID)
}
func (m *mockReviewStore) GetReviewTasks(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID, produtoIDs []uuid.UUID) ([]repository.ReviewTaskRow, error) {
	return m.getReviewTasksFn(ctx, sprintID, equipeID, produtoIDs)
}
func (m *mockReviewStore) GetGDPTCAncestorTaskIDs(ctx context.Context, taskIDs []uuid.UUID) ([]uuid.UUID, error) {
	return m.getGDPTCAncestorIDsFn(ctx, taskIDs)
}
func (m *mockReviewStore) GetReviewPOs(ctx context.Context, equipeID uuid.UUID, produtoIDs []uuid.UUID) ([]repository.ReviewPO, error) {
	return m.getReviewPOsFn(ctx, equipeID, produtoIDs)
}
func (m *mockReviewStore) ListDestaques(ctx context.Context, sprintID, equipeID uuid.UUID) ([]repository.ReviewDestaque, error) {
	return m.listDestaquesFn(ctx, sprintID, equipeID)
}
func (m *mockReviewStore) CreateDestaque(ctx context.Context, d repository.ReviewDestaque) (repository.ReviewDestaque, error) {
	return m.createDestaqueFn(ctx, d)
}
func (m *mockReviewStore) UpdateDestaque(ctx context.Context, id uuid.UUID, titulo, descricao string, link *string) (repository.ReviewDestaque, error) {
	return m.updateDestaqueFn(ctx, id, titulo, descricao, link)
}
func (m *mockReviewStore) DeleteDestaque(ctx context.Context, id uuid.UUID) error {
	return m.deleteDestaqueFn(ctx, id)
}
func (m *mockReviewStore) SaveReviewAnalise(ctx context.Context, a repository.ReviewAnalise) error {
	return m.saveReviewAnaliseFn(ctx, a)
}

// --- mockConfigStore ---

type mockConfigStore struct {
	getConfigFn func(ctx context.Context, chave string) (string, error)
}

func (m *mockConfigStore) GetConfig(ctx context.Context, chave string) (string, error) {
	return m.getConfigFn(ctx, chave)
}

// --- mockEquipeStore ---

type mockEquipeStore struct {
	getMembrosEquipeFn func(ctx context.Context, equipeID uuid.UUID) ([]domain.Membro, error)
}

func (m *mockEquipeStore) GetMembrosEquipe(ctx context.Context, equipeID uuid.UUID) ([]domain.Membro, error) {
	return m.getMembrosEquipeFn(ctx, equipeID)
}

// --- mockDestinatarioStore ---

type mockDestinatarioStore struct {
	getByIDsFn func(ctx context.Context, ids []uuid.UUID) ([]repository.Destinatario, error)
}

func (m *mockDestinatarioStore) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]repository.Destinatario, error) {
	return m.getByIDsFn(ctx, ids)
}

// --- mockSyncScheduleStore ---

type mockSyncScheduleStore struct {
	getDueSchedulesFn func(ctx context.Context, horaMinuto string) ([]domain.SyncSchedule, error)
}

func (m *mockSyncScheduleStore) GetDueSchedules(ctx context.Context, horaMinuto string) ([]domain.SyncSchedule, error) {
	return m.getDueSchedulesFn(ctx, horaMinuto)
}
