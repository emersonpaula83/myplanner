package handler

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
)

func TestDistribuirHorasPorMes_SingleMonth(t *testing.T) {
	projetos := []domain.ProjetoCapacidade{
		{
			DataInicioExecucao: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC),
			DataLimite:         time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
			HorasEquipe:        160,
		},
	}

	result := DistribuirHorasPorMes(projetos, 2026)

	if _, ok := result[3]; !ok {
		t.Fatal("expected hours in March")
	}
	if math.Abs(result[3]-160) > 0.01 {
		t.Errorf("March hours = %.2f, want 160", result[3])
	}
	if result[2] != 0 {
		t.Errorf("February hours = %.2f, want 0", result[2])
	}
}

func TestDistribuirHorasPorMes_SpansTwoMonths(t *testing.T) {
	// Project from March 16 to April 15, 2026
	// March 16-31: 12 weekdays, April 1-15: 11 weekdays, total: 23 weekdays
	projetos := []domain.ProjetoCapacidade{
		{
			DataInicioExecucao: time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
			DataLimite:         time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC),
			HorasEquipe:        230,
		},
	}

	result := DistribuirHorasPorMes(projetos, 2026)

	totalWeekdays := ContarDiasUteis(
		time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC),
	)
	marchWeekdays := ContarDiasUteis(
		time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
	)
	aprilWeekdays := ContarDiasUteis(
		time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC),
	)

	expectedMarch := 230 * float64(marchWeekdays) / float64(totalWeekdays)
	expectedApril := 230 * float64(aprilWeekdays) / float64(totalWeekdays)

	if math.Abs(result[3]-expectedMarch) > 0.01 {
		t.Errorf("March hours = %.2f, want %.2f", result[3], expectedMarch)
	}
	if math.Abs(result[4]-expectedApril) > 0.01 {
		t.Errorf("April hours = %.2f, want %.2f", result[4], expectedApril)
	}
}

func TestDistribuirHorasPorMes_NoOverlap(t *testing.T) {
	projetos := []domain.ProjetoCapacidade{
		{
			DataInicioExecucao: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			DataLimite:         time.Date(2025, 8, 31, 0, 0, 0, 0, time.UTC),
			HorasEquipe:        400,
		},
	}

	result := DistribuirHorasPorMes(projetos, 2026)

	for mes := 1; mes <= 12; mes++ {
		if result[mes] != 0 {
			t.Errorf("Month %d hours = %.2f, want 0", mes, result[mes])
		}
	}
}

func TestDistribuirHorasPorMes_ZeroDuration(t *testing.T) {
	// March 2, 2026 is a Monday (weekday), so this single-day project
	// has diasProjTotal = 1 and should allocate all hours to March.
	projetos := []domain.ProjetoCapacidade{
		{
			DataInicioExecucao: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC),
			DataLimite:         time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC),
			HorasEquipe:        8,
		},
	}

	result := DistribuirHorasPorMes(projetos, 2026)

	if math.Abs(result[3]-8) > 0.01 {
		t.Errorf("March hours = %.2f, want 8", result[3])
	}
}

func TestCalcularCapacidadeMensal_Basic(t *testing.T) {
	ausencias := []domain.AusenciaMensal{
		{MembroID: uuid.New(), Nome: "Alice", Tipo: "ferias", Mes: 7, Dias: 10},
	}
	projetos := []domain.ProjetoCapacidade{
		{
			DataInicioExecucao: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
			DataLimite:         time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC),
			HorasEquipe:        400,
		},
	}

	result := CalcularCapacidadeMensal(2026, 4, ausencias, projetos)

	if len(result) != 12 {
		t.Fatalf("expected 12 months, got %d", len(result))
	}

	jul := result[6] // index 6 = month 7
	if jul.Mes != 7 {
		t.Errorf("Mes = %d, want 7", jul.Mes)
	}

	// July 2026 has 23 weekdays
	// 4 members × 23 × 8 = 736h available, minus Alice 10 × 8 = 80h → 656h
	expectedDisponiveis := 656.0
	if math.Abs(jul.HorasDisponiveis-expectedDisponiveis) > 0.01 {
		t.Errorf("HorasDisponiveis = %.2f, want %.2f", jul.HorasDisponiveis, expectedDisponiveis)
	}

	// All 400h estimated in July (single month project)
	if math.Abs(jul.HorasEstimadas-400) > 0.01 {
		t.Errorf("HorasEstimadas = %.2f, want 400", jul.HorasEstimadas)
	}

	// Delta = (400 - 656) / 656 × 100 ≈ -39.02%
	expectedDelta := ((400.0 - 656.0) / 656.0) * 100
	if math.Abs(jul.PercentualDelta-expectedDelta) > 0.01 {
		t.Errorf("PercentualDelta = %.2f, want %.2f", jul.PercentualDelta, expectedDelta)
	}

	if len(jul.MembrosAusentes) != 1 {
		t.Fatalf("MembrosAusentes count = %d, want 1", len(jul.MembrosAusentes))
	}
	if jul.MembrosAusentes[0].Nome != "Alice" {
		t.Errorf("MembrosAusentes[0].Nome = %q, want Alice", jul.MembrosAusentes[0].Nome)
	}
}

func TestCalcularCapacidadeMensal_NoProjects(t *testing.T) {
	result := CalcularCapacidadeMensal(2026, 3, nil, nil)

	jan := result[0]
	if jan.HorasEstimadas != 0 {
		t.Errorf("HorasEstimadas = %.2f, want 0", jan.HorasEstimadas)
	}
	if jan.HorasDisponiveis <= 0 {
		t.Errorf("HorasDisponiveis = %.2f, want > 0", jan.HorasDisponiveis)
	}
	if jan.PercentualDelta >= 0 {
		t.Errorf("PercentualDelta = %.2f, want < 0 (under capacity)", jan.PercentualDelta)
	}
	if len(jan.MembrosAusentes) != 0 {
		t.Errorf("MembrosAusentes count = %d, want 0", len(jan.MembrosAusentes))
	}
}

func TestCalcularCapacidadeMensal_ZeroMembers(t *testing.T) {
	projetos := []domain.ProjetoCapacidade{
		{
			DataInicioExecucao: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			DataLimite:         time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
			HorasEquipe:        100,
		},
	}

	result := CalcularCapacidadeMensal(2026, 0, nil, projetos)

	jan := result[0]
	if jan.HorasDisponiveis != 0 {
		t.Errorf("HorasDisponiveis = %.2f, want 0", jan.HorasDisponiveis)
	}
	if jan.PercentualDelta != 0 {
		t.Errorf("PercentualDelta = %.2f, want 0 (no division by zero)", jan.PercentualDelta)
	}
}
