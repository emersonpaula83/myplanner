package handler

import (
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
)

func DistribuirHorasPorMes(projetos []domain.ProjetoCapacidade, ano int) map[int]float64 {
	result := make(map[int]float64)
	for _, p := range projetos {
		diasProjTotal := ContarDiasUteis(p.DataInicioExecucao, p.DataLimite)
		if diasProjTotal <= 0 {
			continue
		}
		for mes := 1; mes <= 12; mes++ {
			mesInicio := time.Date(ano, time.Month(mes), 1, 0, 0, 0, 0, time.UTC)
			mesFim := mesInicio.AddDate(0, 1, -1)

			overlapInicio := p.DataInicioExecucao
			if mesInicio.After(overlapInicio) {
				overlapInicio = mesInicio
			}
			overlapFim := p.DataLimite
			if mesFim.Before(overlapFim) {
				overlapFim = mesFim
			}

			if overlapInicio.After(overlapFim) {
				continue
			}

			diasProjMes := ContarDiasUteis(overlapInicio, overlapFim)
			proporcao := float64(diasProjMes) / float64(diasProjTotal)
			result[mes] += p.HorasEquipe * proporcao
		}
	}
	return result
}

func CalcularCapacidadeMensal(
	ano int,
	membrosAtivos int,
	ausencias []domain.AusenciaMensal,
	projetos []domain.ProjetoCapacidade,
) []domain.CapacidadeMes {
	horasEstimadasPorMes := DistribuirHorasPorMes(projetos, ano)

	result := make([]domain.CapacidadeMes, 12)
	for mes := 1; mes <= 12; mes++ {
		mesInicio := time.Date(ano, time.Month(mes), 1, 0, 0, 0, 0, time.UTC)
		mesFim := mesInicio.AddDate(0, 1, -1)
		diasUteis := ContarDiasUteis(mesInicio, mesFim)

		horasDisponiveis := float64(membrosAtivos) * float64(diasUteis) * 8.0

		totalDiasAusencia := 0
		membrosAusentes := make([]domain.MembroAusente, 0)
		for _, a := range ausencias {
			if a.Mes == mes {
				totalDiasAusencia += a.Dias
				membrosAusentes = append(membrosAusentes, domain.MembroAusente{
					Nome: a.Nome,
					Tipo: a.Tipo,
					Dias: a.Dias,
				})
			}
		}
		horasDisponiveis -= float64(totalDiasAusencia) * 8.0
		if horasDisponiveis < 0 {
			horasDisponiveis = 0
		}

		horasEstimadas := horasEstimadasPorMes[mes]

		var percentualDelta float64
		if horasDisponiveis > 0 {
			percentualDelta = ((horasEstimadas - horasDisponiveis) / horasDisponiveis) * 100
		}

		result[mes-1] = domain.CapacidadeMes{
			Mes:              mes,
			HorasDisponiveis: horasDisponiveis,
			HorasEstimadas:   horasEstimadas,
			PercentualDelta:  percentualDelta,
			MembrosAusentes:  membrosAusentes,
		}
	}
	return result
}
