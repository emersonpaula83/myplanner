package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TarefaRepository struct {
	pool *pgxpool.Pool
}

func NewTarefaRepository(pool *pgxpool.Pool) *TarefaRepository {
	return &TarefaRepository{pool: pool}
}

type TarefaListRow struct {
	ID              uuid.UUID  `json:"id"`
	NumeroTicket    string     `json:"numero_ticket"`
	Resumo          string     `json:"resumo"`
	Tipo            string     `json:"tipo"`
	Status          string     `json:"status"`
	TipoDemanda     *string    `json:"tipo_demanda"`
	ResponsavelNome *string    `json:"responsavel_nome"`
	ProdutoNome     *string    `json:"produto_nome"`
	EquipeNome      *string    `json:"equipe_nome"`
	RemovidoEm      *time.Time `json:"removido_em"`
	MotivoRemocao   *string    `json:"motivo_remocao"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type TarefaListFilter struct {
	EquipeID      *uuid.UUID
	ProdutoNome   *string
	ResponsavelID *uuid.UUID
	Removido      string // "sim", "nao", "todos"
	Busca         string
	Page          int
	PerPage       int
}

type TarefaListResult struct {
	Items []TarefaListRow
	Total int
}

func (r *TarefaRepository) ListTarefas(ctx context.Context, f TarefaListFilter) (*TarefaListResult, error) {
	var conditions []string
	var args []any
	argN := 1

	conditions = append(conditions, "t.tipo NOT IN ('Épico', 'Epico')")

	switch f.Removido {
	case "sim":
		conditions = append(conditions, "t.removido_em IS NOT NULL")
	case "todos":
		// no filter
	default:
		conditions = append(conditions, "t.removido_em IS NULL")
	}

	if f.EquipeID != nil {
		conditions = append(conditions, fmt.Sprintf(`t.responsavel_id IN (SELECT membro_id FROM equipe_membros WHERE equipe_id = $%d)`, argN))
		args = append(args, *f.EquipeID)
		argN++
	}

	if f.ProdutoNome != nil {
		conditions = append(conditions, fmt.Sprintf(`EXISTS (SELECT 1 FROM tarefa_produtos tp JOIN produtos p ON p.id = tp.produto_id WHERE tp.tarefa_id = t.id AND LOWER(p.nome) = LOWER($%d))`, argN))
		args = append(args, *f.ProdutoNome)
		argN++
	}

	if f.ResponsavelID != nil {
		conditions = append(conditions, fmt.Sprintf("t.responsavel_id = $%d", argN))
		args = append(args, *f.ResponsavelID)
		argN++
	}

	if f.Busca != "" {
		conditions = append(conditions, fmt.Sprintf("(t.numero_ticket ILIKE $%d OR t.resumo ILIKE $%d)", argN, argN))
		args = append(args, "%"+f.Busca+"%")
		argN++
	}

	where := "WHERE " + strings.Join(conditions, " AND ")

	countQuery := "SELECT COUNT(*) FROM tarefas t " + where
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("counting tarefas: %w", err)
	}

	offset := (f.Page - 1) * f.PerPage
	args = append(args, f.PerPage, offset)

	dataQuery := fmt.Sprintf(`
		SELECT t.id, t.numero_ticket, t.resumo, t.tipo, t.status, t.tipo_demanda,
		       m.nome,
		       (SELECT p.nome FROM tarefa_produtos tp JOIN produtos p ON p.id = tp.produto_id WHERE tp.tarefa_id = t.id LIMIT 1),
		       (SELECT eq.nome FROM equipe_membros em JOIN equipes eq ON eq.id = em.equipe_id WHERE em.membro_id = t.responsavel_id LIMIT 1),
		       t.removido_em, t.motivo_remocao, t.updated_at
		FROM tarefas t
		LEFT JOIN membros m ON m.id = t.responsavel_id
		%s
		ORDER BY t.updated_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argN, argN+1)

	rows, err := r.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("listing tarefas: %w", err)
	}
	defer rows.Close()

	items := make([]TarefaListRow, 0)
	for rows.Next() {
		var row TarefaListRow
		if err := rows.Scan(
			&row.ID, &row.NumeroTicket, &row.Resumo, &row.Tipo, &row.Status, &row.TipoDemanda,
			&row.ResponsavelNome, &row.ProdutoNome, &row.EquipeNome,
			&row.RemovidoEm, &row.MotivoRemocao, &row.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning tarefa: %w", err)
		}
		items = append(items, row)
	}

	return &TarefaListResult{Items: items, Total: total}, rows.Err()
}

func (r *TarefaRepository) HardDeleteTarefa(ctx context.Context, id uuid.UUID) error {
	// Delete from projeto_encerramentos first (no cascade FK)
	_, _ = r.pool.Exec(ctx, `DELETE FROM projeto_encerramentos WHERE epic_id = $1`, id)
	tag, err := r.pool.Exec(ctx, `DELETE FROM tarefas WHERE id = $1 AND removido_em IS NOT NULL`, id)
	if err != nil {
		return fmt.Errorf("hard-deleting tarefa: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("tarefa not found or not marked as removed")
	}
	return nil
}
