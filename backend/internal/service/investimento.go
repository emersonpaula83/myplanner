package service

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type InvestimentoService struct {
	equipeRepo *repository.EquipeRepository
	membroRepo *repository.MembroRepository
	investRepo *repository.InvestimentoRepository
	logger     *zap.Logger
}

func NewInvestimentoService(
	equipeRepo *repository.EquipeRepository,
	membroRepo *repository.MembroRepository,
	investRepo *repository.InvestimentoRepository,
	logger *zap.Logger,
) *InvestimentoService {
	return &InvestimentoService{
		equipeRepo: equipeRepo,
		membroRepo: membroRepo,
		investRepo: investRepo,
		logger:     logger,
	}
}

func (s *InvestimentoService) GetDashboard(ctx context.Context, equipeID uuid.UUID) (*domain.InvestimentoDashboard, error) {
	equipe, err := s.equipeRepo.GetEquipeByID(ctx, equipeID)
	if err != nil {
		return nil, fmt.Errorf("getting equipe: %w", err)
	}
	if equipe == nil {
		return nil, fmt.Errorf("equipe %s not found", equipeID)
	}

	membros, err := s.equipeRepo.GetMembrosEquipe(ctx, equipeID)
	if err != nil {
		return nil, fmt.Errorf("getting membros: %w", err)
	}

	now := time.Now()
	var custoTotal float64
	var temposCasa []int
	var bancoHorasTotal float64
	membrosList := make([]domain.MembroInvestimento, 0, len(membros))

	for _, m := range membros {
		tempoCasa := 0
		if m.DataAdmissao != nil {
			tempoCasa = calcTempoCasaMeses(*m.DataAdmissao, now)
		}
		temposCasa = append(temposCasa, tempoCasa)

		if m.Salario != nil {
			custoTotal += *m.Salario
		}
		if m.BancoHoras != nil {
			bancoHorasTotal += *m.BancoHoras
		}

		topProd, err := s.investRepo.GetTopProdutosMembro(ctx, m.ID, 3)
		if err != nil {
			s.logger.Warn("failed to get top produtos", zap.String("membro", m.Nome), zap.Error(err))
			topProd = []string{}
		}

		var dataAdmStr *string
		if m.DataAdmissao != nil {
			s := m.DataAdmissao.Format("2006-01-02")
			dataAdmStr = &s
		}

		membrosList = append(membrosList, domain.MembroInvestimento{
			ID:             m.ID,
			Nome:           m.Nome,
			AvatarURL:      m.AvatarURL,
			Salario:        m.Salario,
			DataAdmissao:   dataAdmStr,
			TempoCasaMeses: tempoCasa,
			BancoHoras:     m.BancoHoras,
			Cargo:          m.Cargo,
			TopProdutos:    topProd,
		})
	}

	// Sort by salary descending
	sortMembrosBySalarioDesc(membrosList)

	tempoMedio := 0
	if len(temposCasa) > 0 {
		sum := 0
		for _, t := range temposCasa {
			sum += t
		}
		tempoMedio = sum / len(temposCasa)
	}

	custoTotalRef := custoTotal
	return &domain.InvestimentoDashboard{
		Equipe: domain.EquipeInfo{ID: equipeID, Nome: equipe.Nome},
		Sumario: domain.InvestimentoSumario{
			CustoMensalTotal:    &custoTotalRef,
			TotalMembros:        len(membros),
			TempoCasaMedioMeses: tempoMedio,
			BancoHorasTotal:     bancoHorasTotal,
		},
		Membros: membrosList,
	}, nil
}

func (s *InvestimentoService) GetGastosMensais(ctx context.Context, equipeID uuid.UUID, ano int) (*domain.GastosMensaisResponse, error) {
	meses := make([]domain.GastoMensal, 0, 12)

	for mes := 1; mes <= 12; mes++ {
		membros, err := s.investRepo.GetMembrosEquipeNoMes(ctx, equipeID, ano, mes)
		if err != nil {
			return nil, fmt.Errorf("getting membros for month %d: %w", mes, err)
		}

		var custoMes float64
		for _, m := range membros {
			salario, err := s.investRepo.GetSalarioVigenteNoMes(ctx, m.ID, ano, mes)
			if err != nil {
				s.logger.Warn("failed to get salary for month", zap.Int("mes", mes), zap.Error(err))
				continue
			}
			if salario != nil {
				custoMes += *salario
			}
		}

		// Variável nova a cada volta: senão todos os meses apontariam para o
		// mesmo endereço e mostrariam o custo do último mês do laço.
		custoMesRef := math.Round(custoMes*100) / 100
		meses = append(meses, domain.GastoMensal{
			Mes:        mes,
			CustoTotal: &custoMesRef,
		})
	}

	return &domain.GastosMensaisResponse{Ano: ano, Meses: meses}, nil
}

func (s *InvestimentoService) GetAlocacoesProjetos(ctx context.Context, membroID uuid.UUID) (*domain.AlocacoesProjetosResponse, error) {
	projetos, err := s.investRepo.GetAlocacoesProjetos(ctx, membroID)
	if err != nil {
		return nil, fmt.Errorf("getting alocacoes: %w", err)
	}
	if projetos == nil {
		projetos = []domain.ProjetoAlocacao{}
	}
	return &domain.AlocacoesProjetosResponse{Projetos: projetos}, nil
}

func calcTempoCasaMeses(admissao, now time.Time) int {
	years := now.Year() - admissao.Year()
	months := int(now.Month()) - int(admissao.Month())
	if now.Day() < admissao.Day() {
		months--
	}
	total := years*12 + months
	if total < 0 {
		return 0
	}
	return total
}

func sortMembrosBySalarioDesc(membros []domain.MembroInvestimento) {
	for i := 0; i < len(membros); i++ {
		for j := i + 1; j < len(membros); j++ {
			si := 0.0
			sj := 0.0
			if membros[i].Salario != nil {
				si = *membros[i].Salario
			}
			if membros[j].Salario != nil {
				sj = *membros[j].Salario
			}
			if sj > si {
				membros[i], membros[j] = membros[j], membros[i]
			}
		}
	}
}
