package handler

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
)

func TestParsePeriodo(t *testing.T) {
	tests := []struct {
		input string
		valid bool
		meses int
	}{
		{"1m", true, 1},
		{"2m", true, 2},
		{"3m", true, 3},
		{"6m", true, 6},
		{"1a", true, 12},
		{"5m", false, 0},
		{"", false, 0},
		{"abc", false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			inicio, fim, ok := ParsePeriodo(tt.input)
			if ok != tt.valid {
				t.Fatalf("ParsePeriodo(%q) valid = %v, want %v", tt.input, ok, tt.valid)
			}
			if !ok {
				return
			}
			diff := fim.Sub(inicio)
			expectedDays := tt.meses * 30
			if math.Abs(diff.Hours()/24-float64(expectedDays)) > 5 {
				t.Errorf("ParsePeriodo(%q) range = %.0f days, want ~%d", tt.input, diff.Hours()/24, expectedDays)
			}
		})
	}
}

func TestContarDiasUteis(t *testing.T) {
	// Monday 2026-07-06 to Friday 2026-07-10 = 5 weekdays
	inicio := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	fim := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	got := ContarDiasUteis(inicio, fim)
	if got != 5 {
		t.Errorf("ContarDiasUteis Mon-Fri = %d, want 5", got)
	}

	// Monday 2026-07-06 to Sunday 2026-07-12 = 5 weekdays
	fim2 := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	got2 := ContarDiasUteis(inicio, fim2)
	if got2 != 5 {
		t.Errorf("ContarDiasUteis Mon-Sun = %d, want 5", got2)
	}

	// Two full weeks: Mon 2026-07-06 to Fri 2026-07-17 = 10 weekdays
	fim3 := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	got3 := ContarDiasUteis(inicio, fim3)
	if got3 != 10 {
		t.Errorf("ContarDiasUteis 2 weeks = %d, want 10", got3)
	}
}

func TestCalcularResumoEquipe(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()

	membros := []domain.Membro{
		{ID: id1, Nome: "Alice"},
		{ID: id2, Nome: "Bob"},
	}

	ausencias := map[uuid.UUID]int{
		id1: 2, // 2 weekdays absent
		// id2: 0 (no absences)
	}

	tarefas := []domain.HorasTarefasMembro{
		{
			MembroID:             id1,
			TotalSegundos:        144000, // 40h
			SegundosMetas:        72000,  // 20h
			SegundosCompromissos: 36000,  // 10h
			SegundosIniciativas:  36000,  // 10h
			SegundosManutencao:   7200,   // 2h
			SegundosMelhorias:    25200,  // 7h (melhorias+inovação)
			SegundosSuporte:      3600,   // 1h
		},
		{
			MembroID:             id2,
			TotalSegundos:        72000, // 20h
			SegundosMetas:        36000, // 10h
			SegundosCompromissos: 18000, // 5h
			SegundosIniciativas:  18000, // 5h
			SegundosManutencao:   3600,  // 1h
			SegundosMelhorias:    10800, // 3h (melhorias+inovação)
			SegundosSuporte:      3600,  // 1h
		},
	}

	// 10 weekdays = 80h total per person
	diasUteis := 10

	teamID := uuid.New()
	resumo := CalcularResumoEquipe(teamID, "TeamA", "2m", membros, ausencias, tarefas, diasUteis)

	if resumo.NomeEquipe != "TeamA" {
		t.Errorf("NomeEquipe = %q, want TeamA", resumo.NomeEquipe)
	}

	// Alice: horasReais = 80 - 2*8 = 64h, horasCards = 40h, atuacao = 40/64*100 = 62.5%
	// Bob:   horasReais = 80 - 0   = 80h, horasCards = 20h, atuacao = 20/80*100 = 25.0%
	// Media: (62.5 + 25.0) / 2 = 43.75%
	if math.Abs(resumo.AtuacaoRastreada-43.75) > 0.01 {
		t.Errorf("AtuacaoRastreada = %.2f, want 43.75", resumo.AtuacaoRastreada)
	}

	// Total seconds: 144000 + 72000 = 216000
	// Metas: (72000+36000)/216000*100 = 50%
	if math.Abs(resumo.PercentualMetas-50.0) > 0.01 {
		t.Errorf("PercentualMetas = %.2f, want 50.00", resumo.PercentualMetas)
	}

	// Compromissos: (36000+18000)/216000*100 = 25%
	if math.Abs(resumo.PercentualCompromissos-25.0) > 0.01 {
		t.Errorf("PercentualCompromissos = %.2f, want 25.00", resumo.PercentualCompromissos)
	}

	// Iniciativas: (36000+18000)/216000*100 = 25%
	if math.Abs(resumo.PercentualIniciativas-25.0) > 0.01 {
		t.Errorf("PercentualIniciativas = %.2f, want 25.00", resumo.PercentualIniciativas)
	}

	// Sub-breakdown within Iniciativas (total = 54000)
	// Manutencao: (7200+3600)/54000*100 = 20%
	if math.Abs(resumo.DetalhesIniciativas.PercentualManutencao-20.0) > 0.01 {
		t.Errorf("PercentualManutencao = %.2f, want 20.00", resumo.DetalhesIniciativas.PercentualManutencao)
	}

	// Melhorias e Inovação: (25200+10800)/54000*100 ≈ 66.67%
	if math.Abs(resumo.DetalhesIniciativas.PercentualMelhorias-66.67) > 0.01 {
		t.Errorf("PercentualMelhorias = %.2f, want 66.67", resumo.DetalhesIniciativas.PercentualMelhorias)
	}

	// Suporte: (3600+3600)/54000*100 ≈ 13.33%
	if math.Abs(resumo.DetalhesIniciativas.PercentualSuporte-13.33) > 0.01 {
		t.Errorf("PercentualSuporte = %.2f, want 13.33", resumo.DetalhesIniciativas.PercentualSuporte)
	}

	if len(resumo.Membros) != 2 {
		t.Fatalf("Membros count = %d, want 2", len(resumo.Membros))
	}
}

func TestCalcularResumoEquipe_NoTasks(t *testing.T) {
	id1 := uuid.New()
	membros := []domain.Membro{{ID: id1, Nome: "Solo"}}
	ausencias := map[uuid.UUID]int{}
	tarefas := []domain.HorasTarefasMembro{}

	resumo := CalcularResumoEquipe(uuid.New(), "Empty", "1m", membros, ausencias, tarefas, 22)

	if resumo.AtuacaoRastreada != 0 {
		t.Errorf("AtuacaoRastreada = %.2f, want 0", resumo.AtuacaoRastreada)
	}
	if resumo.PercentualMetas != 0 {
		t.Errorf("PercentualMetas = %.2f, want 0", resumo.PercentualMetas)
	}
}

func TestCalcularResumoEquipe_AllAbsent(t *testing.T) {
	id1 := uuid.New()
	membros := []domain.Membro{{ID: id1, Nome: "Absent"}}
	ausencias := map[uuid.UUID]int{id1: 22} // all days absent
	tarefas := []domain.HorasTarefasMembro{
		{MembroID: id1, TotalSegundos: 3600},
	}

	resumo := CalcularResumoEquipe(uuid.New(), "Team", "1m", membros, ausencias, tarefas, 22)

	// horasReais = 0, so atuacao = 0 (no division by zero)
	if resumo.AtuacaoRastreada != 0 {
		t.Errorf("AtuacaoRastreada = %.2f, want 0", resumo.AtuacaoRastreada)
	}
}
