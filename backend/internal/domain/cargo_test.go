package domain

import "testing"

func TestIsPromocaoValida(t *testing.T) {
	tests := []struct {
		atual, novo string
		want        bool
	}{
		{CargoAnalistaI, CargoAnalistaII, true},
		{CargoAnalistaII, CargoAnalistaIII, true},
		{CargoAnalistaIII, CargoEspecialistaI, true},
		{CargoAnalistaIII, CargoCoordenadorDesenvolvimento, true},
		{CargoEspecialistaI, CargoEspecialistaII, true},
		{CargoEspecialistaI, CargoLiderTecnico, true},
		{CargoEspecialistaII, CargoMaster, true},
		{CargoEspecialistaII, CargoLiderTecnico, true},
		{CargoCoordenadorDesenvolvimento, CargoLiderTecnico, true},
		// Invalid promotions
		{CargoAnalistaI, CargoAnalistaIII, false},
		{CargoAnalistaII, CargoEspecialistaI, false},
		{CargoMaster, CargoLiderTecnico, false},
		{CargoLiderTecnico, CargoMaster, false},
		{"", CargoAnalistaI, false},
		{CargoAnalistaI, CargoAnalistaI, false},
	}
	for _, tt := range tests {
		t.Run(tt.atual+"->"+tt.novo, func(t *testing.T) {
			got := IsPromocaoValida(tt.atual, tt.novo)
			if got != tt.want {
				t.Errorf("IsPromocaoValida(%q, %q) = %v, want %v", tt.atual, tt.novo, got, tt.want)
			}
		})
	}
}
