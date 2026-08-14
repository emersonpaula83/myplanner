package service

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestCalcStdDev(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		want   float64
	}{
		{"nil slice", nil, 0},
		{"empty slice", []float64{}, 0},
		{"all same values", []float64{5, 5, 5}, 0},
		{"known values", []float64{2, 4, 4, 4, 5, 5, 7, 9}, 2.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calcStdDev(tt.values)
			if math.Abs(got-tt.want) > 0.01 {
				t.Errorf("calcStdDev(%v) = %v, want ≈%v", tt.values, got, tt.want)
			}
		})
	}
}

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want string
	}{
		{"a non-empty", "a", "b", "a"},
		{"a empty, b non-empty", "", "b", "b"},
		{"both empty", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstNonEmpty(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("firstNonEmpty(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestResetHorasMov(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	original := map[uuid.UUID]*membroState{
		id1: {mc: MembroCapacity{Nome: "Alice"}, horasMov: 5.0, role: roleDoador},
		id2: {mc: MembroCapacity{Nome: "Bob"}, horasMov: -3.0, role: roleNeutral},
	}

	fresh := resetHorasMov(original)

	if len(fresh) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(fresh))
	}

	st1, ok := fresh[id1]
	if !ok {
		t.Fatal("expected id1 in fresh map")
	}
	if st1.horasMov != 0 {
		t.Errorf("id1 horasMov = %v, want 0", st1.horasMov)
	}
	if st1.role != roleDoador {
		t.Errorf("id1 role = %v, want roleDoador", st1.role)
	}
	if st1.mc.Nome != "Alice" {
		t.Errorf("id1 mc.Nome = %q, want %q", st1.mc.Nome, "Alice")
	}

	st2, ok := fresh[id2]
	if !ok {
		t.Fatal("expected id2 in fresh map")
	}
	if st2.horasMov != 0 {
		t.Errorf("id2 horasMov = %v, want 0", st2.horasMov)
	}
	if st2.role != roleNeutral {
		t.Errorf("id2 role = %v, want roleNeutral", st2.role)
	}

	// Verify original map was not mutated (fresh copy, distinct pointers).
	if original[id1].horasMov != 5.0 {
		t.Errorf("original id1 horasMov mutated: got %v, want 5.0", original[id1].horasMov)
	}
	if original[id1] == fresh[id1] {
		t.Error("expected fresh map to contain new *membroState pointers, not shared with original")
	}
}

func TestBuildEqualizerPrompt(t *testing.T) {
	membros := []aiMembroInput{
		{Nome: "Alice", PctAlocacao: 120, HorasDisponiveis: 40, HorasAlocadas: 48},
		{Nome: "Bob", PctAlocacao: 60, HorasDisponiveis: 40, HorasAlocadas: 24},
	}
	tarefas := []aiTarefaInput{
		{Ticket: "PROJ-1", Resumo: "Fix bug", Horas: 8, Tipo: "Bug", ResponsavelNome: "Alice"},
	}

	system, user := buildEqualizerPrompt(membros, tarefas)

	if system == "" {
		t.Error("expected non-empty system prompt")
	}
	if user == "" {
		t.Error("expected non-empty user prompt")
	}
	if !strings.Contains(user, "DADOS DA SPRINT:") {
		t.Error("expected user prompt to contain header")
	}

	// The user prompt is "DADOS DA SPRINT:\n<json>" — extract and validate the JSON.
	jsonPart := strings.TrimPrefix(user, "DADOS DA SPRINT:\n")
	var data struct {
		Membros []aiMembroInput `json:"membros"`
		Tarefas []aiTarefaInput `json:"tarefas"`
	}
	if err := json.Unmarshal([]byte(jsonPart), &data); err != nil {
		t.Fatalf("user prompt JSON payload did not parse: %v", err)
	}
	if len(data.Membros) != 2 {
		t.Errorf("expected 2 membros in payload, got %d", len(data.Membros))
	}
	if len(data.Tarefas) != 1 {
		t.Errorf("expected 1 tarefa in payload, got %d", len(data.Tarefas))
	}
	if data.Membros[0].Nome != "Alice" {
		t.Errorf("expected first membro Nome = Alice, got %q", data.Membros[0].Nome)
	}
	if data.Tarefas[0].Ticket != "PROJ-1" {
		t.Errorf("expected first tarefa Ticket = PROJ-1, got %q", data.Tarefas[0].Ticket)
	}
}

func TestNadaASugerir(t *testing.T) {
	svc := &EqualizerService{}
	cap := &SprintCapacityResult{
		Membros: []MembroCapacity{
			{MembroID: uuid.New(), Nome: "Alice", DaEquipe: true, HorasAlocadas: 40, HorasDisponiveis: 80, PercentualAlocacao: 50},
		},
	}
	states := map[uuid.UUID]*membroState{}
	result := svc.nadaASugerir(cap, states, "motivo teste")
	if !result.NadaASugerir {
		t.Error("expected NadaASugerir = true")
	}
	if result.Motivo != "motivo teste" {
		t.Errorf("expected motivo 'motivo teste', got %q", result.Motivo)
	}
	if len(result.MembrosAntesDepois) != 1 {
		t.Errorf("expected 1 membro antes/depois, got %d", len(result.MembrosAntesDepois))
	}
}

func TestBuildMembrosAntesDepois(t *testing.T) {
	svc := &EqualizerService{}
	id1 := uuid.New()
	id2 := uuid.New()
	cap := &SprintCapacityResult{
		Membros: []MembroCapacity{
			{MembroID: id1, Nome: "Alice", DaEquipe: true, HorasAlocadas: 40, HorasDisponiveis: 80, PercentualAlocacao: 50},
			{MembroID: id2, Nome: "Bob", DaEquipe: true, Desligado: true}, // should be excluded
			{MembroID: uuid.New(), Nome: "Carol", DaEquipe: false},        // not from team, excluded
		},
	}
	states := map[uuid.UUID]*membroState{
		id1: {horasMov: -10.0}, // donated 10 hours
	}
	result := svc.buildMembrosAntesDepois(cap, states)
	if len(result) != 1 {
		t.Fatalf("expected 1 member (Alice only), got %d", len(result))
	}
	if result[0].Nome != "Alice" {
		t.Errorf("expected Alice, got %s", result[0].Nome)
	}
	if result[0].HorasDepois != 30 { // 40 + (-10) = 30
		t.Errorf("expected HorasDepois=30, got %f", result[0].HorasDepois)
	}
	expectedPct := 30.0 / 80.0 * 100 // 37.5%
	if result[0].PctDepois != expectedPct {
		t.Errorf("expected PctDepois=%f, got %f", expectedPct, result[0].PctDepois)
	}
}

func TestBuildMembrosAntesDepois_NilState(t *testing.T) {
	svc := &EqualizerService{}
	id1 := uuid.New()
	cap := &SprintCapacityResult{
		Membros: []MembroCapacity{
			{MembroID: id1, Nome: "Alice", DaEquipe: true, HorasAlocadas: 40, HorasDisponiveis: 80, PercentualAlocacao: 50},
		},
	}
	// no states → horasDepois should equal HorasAlocadas
	result := svc.buildMembrosAntesDepois(cap, map[uuid.UUID]*membroState{})
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	if result[0].HorasDepois != 40 {
		t.Errorf("expected HorasDepois=40 (unchanged), got %f", result[0].HorasDepois)
	}
}
