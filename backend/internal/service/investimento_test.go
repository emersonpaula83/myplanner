package service

import (
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
)

func TestCalcTempoCasaMeses(t *testing.T) {
	tests := []struct {
		name     string
		admissao time.Time
		now      time.Time
		want     int
	}{
		{
			name:     "exact 2 years",
			admissao: time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
			want:     24,
		},
		{
			name:     "partial month",
			admissao: time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
			want:     28,
		},
		{
			name:     "same month",
			admissao: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
			want:     0,
		},
		{
			name:     "future admission returns 0",
			admissao: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calcTempoCasaMeses(tt.admissao, tt.now)
			if got != tt.want {
				t.Errorf("calcTempoCasaMeses() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSortMembrosBySalarioDesc(t *testing.T) {
	s1 := 5000.0
	s2 := 12000.0
	s3 := 8000.0

	membros := []domain.MembroInvestimento{
		{Nome: "A", Salario: &s1},
		{Nome: "B", Salario: &s2},
		{Nome: "C", Salario: &s3},
		{Nome: "D", Salario: nil},
	}

	sortMembrosBySalarioDesc(membros)

	if membros[0].Nome != "B" {
		t.Errorf("first = %s, want B (12000)", membros[0].Nome)
	}
	if membros[1].Nome != "C" {
		t.Errorf("second = %s, want C (8000)", membros[1].Nome)
	}
	if membros[2].Nome != "A" {
		t.Errorf("third = %s, want A (5000)", membros[2].Nome)
	}
	if membros[3].Nome != "D" {
		t.Errorf("fourth = %s, want D (nil)", membros[3].Nome)
	}
}
