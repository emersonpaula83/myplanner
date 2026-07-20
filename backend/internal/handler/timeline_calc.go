package handler

import (
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
)

func ContarDiasUteisComFeriadosSet(inicio, fim time.Time, feriadoSet map[string]bool) int {
	dias := 0
	for d := inicio; !d.After(fim); d = d.AddDate(0, 0, 1) {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		if feriadoSet[d.Format("2006-01-02")] {
			continue
		}
		dias++
	}
	return dias
}

func CalcularCapacidadeMensal(
	ano int,
	membrosAtivos int,
	ausencias []domain.AusenciaMensal,
	feriados []time.Time,
) []domain.CapacidadeMes {
	feriadoSet := make(map[string]bool)
	for _, f := range feriados {
		feriadoSet[f.Format("2006-01-02")] = true
	}

	result := make([]domain.CapacidadeMes, 12)
	for mes := 1; mes <= 12; mes++ {
		mesInicio := time.Date(ano, time.Month(mes), 1, 0, 0, 0, 0, time.UTC)
		mesFim := mesInicio.AddDate(0, 1, -1)
		diasUteis := ContarDiasUteisComFeriadosSet(mesInicio, mesFim, feriadoSet)

		horasMaximas := float64(membrosAtivos) * float64(diasUteis) * 6.0

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
		horasDisponiveis := horasMaximas - float64(totalDiasAusencia)*6.0
		if horasDisponiveis < 0 {
			horasDisponiveis = 0
		}

		var percentualCap float64
		if horasMaximas > 0 {
			percentualCap = (horasDisponiveis / horasMaximas) * 100
		}

		result[mes-1] = domain.CapacidadeMes{
			Mes:              mes,
			HorasDisponiveis: horasDisponiveis,
			HorasMaximas:     horasMaximas,
			PercentualCap:    percentualCap,
			MembrosAusentes:  membrosAusentes,
		}
	}
	return result
}
