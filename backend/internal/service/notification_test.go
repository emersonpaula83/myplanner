package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractAnaliseText(t *testing.T) {
	t.Run("valid JSON", func(t *testing.T) {
		raw := json.RawMessage(`{"analises_por_produto":[{"produto":"Prod A","foco_sprint":{"descricao":"Focus on X"},"top3_entregas":[{"ticket":"T-1","resumo":"Task 1"}]}]}`)
		result := extractAnaliseText(raw)
		if !strings.Contains(result, "Prod A") {
			t.Errorf("expected result to contain %q, got %q", "Prod A", result)
		}
		if !strings.Contains(result, "Focus on X") {
			t.Errorf("expected result to contain %q, got %q", "Focus on X", result)
		}
		if !strings.Contains(result, "T-1") {
			t.Errorf("expected result to contain %q, got %q", "T-1", result)
		}
		if !strings.Contains(result, "Task 1") {
			t.Errorf("expected result to contain %q, got %q", "Task 1", result)
		}
	})

	t.Run("multiple produtos and entregas", func(t *testing.T) {
		raw := json.RawMessage(`{"analises_por_produto":[
			{"produto":"Prod A","foco_sprint":{"descricao":"Focus A"},"top3_entregas":[{"ticket":"T-1","resumo":"R1"},{"ticket":"T-2","resumo":"R2"}]},
			{"produto":"Prod B","foco_sprint":{"descricao":"Focus B"},"top3_entregas":[{"ticket":"T-3","resumo":"R3"}]}
		]}`)
		result := extractAnaliseText(raw)
		for _, want := range []string{"Prod A", "Focus A", "T-1", "R1", "T-2", "R2", "Prod B", "Focus B", "T-3", "R3"} {
			if !strings.Contains(result, want) {
				t.Errorf("expected result to contain %q, got %q", want, result)
			}
		}
	})

	t.Run("invalid JSON returns raw string", func(t *testing.T) {
		raw := json.RawMessage(`not json`)
		result := extractAnaliseText(raw)
		if result != "not json" {
			t.Errorf("expected raw string %q, got %q", "not json", result)
		}
	})

	t.Run("empty array returns raw string", func(t *testing.T) {
		raw := json.RawMessage(`{"analises_por_produto":[]}`)
		result := extractAnaliseText(raw)
		if result != string(raw) {
			t.Errorf("expected raw string %q, got %q", string(raw), result)
		}
	})

	t.Run("missing field returns raw string", func(t *testing.T) {
		raw := json.RawMessage(`{"outro_campo":"valor"}`)
		result := extractAnaliseText(raw)
		if result != string(raw) {
			t.Errorf("expected raw string %q, got %q", string(raw), result)
		}
	})
}
