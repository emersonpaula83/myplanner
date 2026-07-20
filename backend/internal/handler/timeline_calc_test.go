package handler

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
)

func TestCalcularCapacidadeMensal_Basic(t *testing.T) {
	ausencias := []domain.AusenciaMensal{
		{MembroID: uuid.New(), Nome: "Alice", Tipo: "ferias", Mes: 7, Dias: 10},
	}

	result := CalcularCapacidadeMensal(2026, 4, ausencias, nil)

	if len(result) != 12 {
		t.Fatalf("expected 12 months, got %d", len(result))
	}

	jul := result[6]
	if jul.Mes != 7 {
		t.Errorf("Mes = %d, want 7", jul.Mes)
	}

	// July 2026 has 23 weekdays
	// Max: 4 × 23 × 6 = 552h, minus Alice 10 × 6 = 60h → 492h
	expectedMax := 4.0 * 23.0 * 6.0
	expectedDisp := expectedMax - 60.0
	if math.Abs(jul.HorasDisponiveis-expectedDisp) > 0.01 {
		t.Errorf("HorasDisponiveis = %.2f, want %.2f", jul.HorasDisponiveis, expectedDisp)
	}
	if math.Abs(jul.HorasMaximas-expectedMax) > 0.01 {
		t.Errorf("HorasMaximas = %.2f, want %.2f", jul.HorasMaximas, expectedMax)
	}

	expectedPct := (expectedDisp / expectedMax) * 100
	if math.Abs(jul.PercentualCap-expectedPct) > 0.01 {
		t.Errorf("PercentualCap = %.2f, want %.2f", jul.PercentualCap, expectedPct)
	}

	if len(jul.MembrosAusentes) != 1 || jul.MembrosAusentes[0].Nome != "Alice" {
		t.Errorf("MembrosAusentes unexpected: %+v", jul.MembrosAusentes)
	}
}

func TestCalcularCapacidadeMensal_WithFeriados(t *testing.T) {
	feriados := []time.Time{
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	result := CalcularCapacidadeMensal(2026, 2, nil, feriados)

	jan := result[0]
	// Jan 2026: 22 weekdays, minus 1 feriado (Jan 1 is Thursday) = 21
	// Max: 2 × 21 × 6 = 252h
	expectedMax := 2.0 * 21.0 * 6.0
	if math.Abs(jan.HorasMaximas-expectedMax) > 0.01 {
		t.Errorf("HorasMaximas = %.2f, want %.2f", jan.HorasMaximas, expectedMax)
	}
	if math.Abs(jan.PercentualCap-100) > 0.01 {
		t.Errorf("PercentualCap = %.2f, want 100", jan.PercentualCap)
	}
}

func TestCalcularCapacidadeMensal_ZeroMembers(t *testing.T) {
	result := CalcularCapacidadeMensal(2026, 0, nil, nil)

	jan := result[0]
	if jan.HorasDisponiveis != 0 {
		t.Errorf("HorasDisponiveis = %.2f, want 0", jan.HorasDisponiveis)
	}
	if jan.PercentualCap != 0 {
		t.Errorf("PercentualCap = %.2f, want 0", jan.PercentualCap)
	}
}
