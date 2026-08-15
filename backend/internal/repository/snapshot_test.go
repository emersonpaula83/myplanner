package repository

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestSprintSnapshotV2_MarshalRoundTrip(t *testing.T) {
	membroID := uuid.New()
	taskID := uuid.New()

	original := SprintSnapshotV2{
		Version: 2,
		Tarefas: []SnapshotTask{
			{
				ID:              taskID,
				NumeroTicket:    "PROJ-1",
				Resumo:          "Test task",
				Tipo:            "Story",
				TipoDemanda:     "Evolutiva",
				Status:          "Concluído",
				NaoPlanejada:    false,
				EstimativaTempo: intPtr(14400),
				Produtos:        []string{"Produto A"},
				ProdutoIDs:      []uuid.UUID{uuid.New()},
			},
		},
		Capacidade: &SnapshotCapacity{
			DiasUteis:              10,
			HorasTotalSprint:       480.0,
			HorasAlocadas:          320.0,
			HorasExecutadas:        200.0,
			HorasPendentesExecucao: 80.0,
			TotalMembros:           1,
			Membros: []SnapshotMembroCapacity{
				{
					MembroID:         membroID,
					Nome:             "Dev 1",
					HorasAlocadas:    32.0,
					HorasExecutadas:  20.0,
					HorasDisponiveis: 48.0,
				},
			},
		},
		Burndown: &SnapshotBurndown{
			HorasTotal: 320.0,
			LinhaIdeal: []SnapshotBurndownPoint{
				{Data: "2026-01-05", Horas: 320.0},
				{Data: "2026-01-06", Horas: 288.0},
			},
			LinhaReal: []SnapshotBurndownPoint{
				{Data: "2026-01-05", Horas: 310.0},
			},
			LinhaUnplanned: []SnapshotBurndownPoint{
				{Data: "2026-01-05", Horas: 0},
				{Data: "2026-01-06", Horas: 8.0},
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded SprintSnapshotV2
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Version != 2 {
		t.Errorf("expected version 2, got %d", decoded.Version)
	}
	if len(decoded.Tarefas) != 1 {
		t.Fatalf("expected 1 tarefa, got %d", len(decoded.Tarefas))
	}
	if decoded.Tarefas[0].NumeroTicket != "PROJ-1" {
		t.Errorf("expected PROJ-1, got %s", decoded.Tarefas[0].NumeroTicket)
	}
	if decoded.Capacidade == nil {
		t.Fatal("expected capacidade non-nil")
	}
	if decoded.Capacidade.DiasUteis != 10 {
		t.Errorf("expected 10 dias uteis, got %d", decoded.Capacidade.DiasUteis)
	}
	if decoded.Capacidade.HorasAlocadas != 320.0 {
		t.Errorf("expected 320.0 horas alocadas, got %v", decoded.Capacidade.HorasAlocadas)
	}
	if len(decoded.Capacidade.Membros) != 1 {
		t.Fatalf("expected 1 membro, got %d", len(decoded.Capacidade.Membros))
	}
	if decoded.Capacidade.Membros[0].Nome != "Dev 1" {
		t.Errorf("expected 'Dev 1', got %s", decoded.Capacidade.Membros[0].Nome)
	}
	if decoded.Burndown == nil {
		t.Fatal("expected burndown non-nil")
	}
	if decoded.Burndown.HorasTotal != 320.0 {
		t.Errorf("expected 320.0, got %v", decoded.Burndown.HorasTotal)
	}
	if len(decoded.Burndown.LinhaIdeal) != 2 {
		t.Errorf("expected 2 ideal points, got %d", len(decoded.Burndown.LinhaIdeal))
	}
}

func TestSprintSnapshotV2_BackwardCompatV1(t *testing.T) {
	// V1 snapshots are just []SnapshotTask (same fields as ReviewTaskRow)
	v1Data := `[
		{
			"id": "` + uuid.New().String() + `",
			"numero_ticket": "OLD-1",
			"resumo": "Legacy task",
			"tipo": "Bug",
			"tipo_demanda": "Corretiva",
			"status": "Concluído",
			"nao_planejada": false,
			"produtos": ["Produto X"],
			"produto_ids": ["` + uuid.New().String() + `"]
		}
	]`

	// Trying to unmarshal as V2 should fail (no version field) or have version 0
	var v2 SprintSnapshotV2
	err := json.Unmarshal([]byte(v1Data), &v2)

	// V1 is an array, V2 is an object — unmarshal into V2 struct should fail
	if err == nil && v2.Version >= 2 {
		t.Fatal("V1 data should NOT parse as valid V2")
	}

	// V1 should parse as []ReviewTaskRow
	var tasks []ReviewTaskRow
	if err := json.Unmarshal([]byte(v1Data), &tasks); err != nil {
		t.Fatalf("V1 unmarshal failed: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].NumeroTicket != "OLD-1" {
		t.Errorf("expected OLD-1, got %s", tasks[0].NumeroTicket)
	}
}

func TestSprintSnapshotV2_GetSprintSnapshotCompat(t *testing.T) {
	// Simulates the logic in GetSprintSnapshot for both v1 and v2

	t.Run("v2 format", func(t *testing.T) {
		v2 := SprintSnapshotV2{
			Version: 2,
			Tarefas: []SnapshotTask{
				{NumeroTicket: "V2-1", Resumo: "V2 task", Status: "Concluído"},
			},
			Capacidade: &SnapshotCapacity{DiasUteis: 10},
		}
		raw, _ := json.Marshal(v2)

		var decoded SprintSnapshotV2
		err := json.Unmarshal(raw, &decoded)
		if err != nil || decoded.Version < 2 {
			t.Fatal("should parse as v2")
		}
		if len(decoded.Tarefas) != 1 || decoded.Tarefas[0].NumeroTicket != "V2-1" {
			t.Errorf("unexpected tarefas: %+v", decoded.Tarefas)
		}
	})

	t.Run("v1 format fallback", func(t *testing.T) {
		tasks := []SnapshotTask{
			{NumeroTicket: "V1-1", Resumo: "V1 task", Status: "Desenvolvimento"},
		}
		raw, _ := json.Marshal(tasks)

		var v2 SprintSnapshotV2
		err := json.Unmarshal(raw, &v2)
		isV2 := err == nil && v2.Version >= 2

		if isV2 {
			t.Fatal("V1 data should not parse as valid V2")
		}

		// Fallback: parse as array
		var fallback []SnapshotTask
		if err := json.Unmarshal(raw, &fallback); err != nil {
			t.Fatalf("v1 fallback failed: %v", err)
		}
		if len(fallback) != 1 || fallback[0].NumeroTicket != "V1-1" {
			t.Errorf("unexpected fallback: %+v", fallback)
		}
	})
}

func TestSnapshotCapacity_NilFields(t *testing.T) {
	snapshot := SprintSnapshotV2{
		Version: 2,
		Tarefas: []SnapshotTask{},
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded SprintSnapshotV2
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Capacidade != nil {
		t.Error("expected nil capacidade")
	}
	if decoded.Burndown != nil {
		t.Error("expected nil burndown")
	}
}

func intPtr(v int) *int {
	return &v
}
