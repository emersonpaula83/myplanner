package service

import (
	"testing"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
)

func TestParseSalarioBR(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    *float64
		wantErr bool
	}{
		{"simple value", "R$ 6.480,00", floatPtr(6480.00), false},
		{"no thousands separator", "R$ 950,50", floatPtr(950.50), false},
		{"large value", "R$ 12.500,00", floatPtr(12500.00), false},
		{"dash means null", "-", nil, false},
		{"empty means null", "", nil, false},
		{"invalid", "abc", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSalarioBR(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.want == nil {
				if got != nil {
					t.Errorf("got %v, want nil", *got)
				}
				return
			}
			if got == nil || *got != *tt.want {
				t.Errorf("got %v, want %v", got, *tt.want)
			}
		})
	}
}

func TestParseDataPlanilha(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    *string
		wantErr bool
	}{
		{"dd/mm/yyyy", "18/05/2026", strPtr("2026-05-18"), false},
		{"excel serial", "46083", strPtr("2026-03-02"), false},
		{"dash means null", "-", nil, false},
		{"empty means null", "", nil, false},
		{"invalid", "not-a-date", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDataPlanilha(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.want == nil {
				if got != nil {
					t.Errorf("got %v, want nil", *got)
				}
				return
			}
			if got == nil || *got != *tt.want {
				t.Errorf("got %v, want %v", got, *tt.want)
			}
		})
	}
}

func TestExtractCargoNivel(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want *string
	}{
		{"analista i", "ANALISTA I DE SUPORTE", strPtr(domain.CargoAnalistaI)},
		{"analista i end of string", "ANALISTA DE SUPORTE ANALISTA I", strPtr(domain.CargoAnalistaI)},
		{"analista ii", "ANALISTA II DE DESENVOLVIMENTO CLOUD", strPtr(domain.CargoAnalistaII)},
		{"analista iii", "ANALISTA III DE DADOS", strPtr(domain.CargoAnalistaIII)},
		{"especialista i", "ESPECIALISTA I DE DADOS", strPtr(domain.CargoEspecialistaI)},
		{"especialista ii", "ESPECIALISTA II DE DADOS", strPtr(domain.CargoEspecialistaII)},
		{"master", "MASTER DE ENGENHARIA", strPtr(domain.CargoMaster)},
		{"coordenador", "COORDENADOR DE TECNOLOGIA", strPtr(domain.CargoCoordenadorDesenvolvimento)},
		{"lider accented", "LÍDER TÉCNICO", strPtr(domain.CargoLiderTecnico)},
		{"lider unaccented", "LIDER DE EQUIPE", strPtr(domain.CargoLiderTecnico)},
		{"tecnico accented", "TÉCNICO DE SUPORTE", strPtr(domain.CargoAnalistaI)},
		{"tecnico unaccented", "TECNICO DE INFRAESTRUTURA", strPtr(domain.CargoAnalistaI)},
		{"no match", "GERENTE DE PROJETOS", nil},
		{"empty", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractCargoNivel(tt.in)
			if tt.want == nil {
				if got != nil {
					t.Errorf("got %v, want nil", *got)
				}
				return
			}
			if got == nil || *got != *tt.want {
				t.Errorf("got %v, want %v", got, *tt.want)
			}
		})
	}
}

func TestParseCSVPlanilha(t *testing.T) {
	csvContent := "Nome,Gestão,Time / Squad,Função,Matrícula,Admissão,Salário,Último Aumento\n" +
		"RICARDO KAZUO DINIZ NOZAKI,Angela Kanegae Oda,DEVOPS RM,ANALISTA II DE DESENVOLVIMENTO CLOUD,000101016701,18/05/2026,\"R$ 6.480,00\",01/01/2026\n" +
		"SUB 167064 - AGILISTA,Angela Kanegae Oda,DEVOPS RM,AGILISTA,-,-,\"R$ 0,00\",-\n" +
		"FULANO DE TAL,Novo Gestor,DEVOPS NOVA,ESPECIALISTA I DE DADOS,-,10/03/2026,\"R$ 8.000,00\",46083\n" +
		"3,,,,,,,\n"

	result, err := ParseCSVPlanilha(csvContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Linhas) != 2 {
		t.Fatalf("got %d linhas, want 2", len(result.Linhas))
	}
	if len(result.Ignorados) != 2 {
		t.Fatalf("got %d ignorados, want 2", len(result.Ignorados))
	}
	if result.Ignorados[0].Motivo != "SUB" {
		t.Errorf("ignorados[0].Motivo = %q, want SUB", result.Ignorados[0].Motivo)
	}
	if result.Ignorados[1].Motivo != "total" {
		t.Errorf("ignorados[1].Motivo = %q, want total", result.Ignorados[1].Motivo)
	}

	ricardo := result.Linhas[0]
	if ricardo.Nome != "RICARDO KAZUO DINIZ NOZAKI" {
		t.Errorf("ricardo.Nome = %q", ricardo.Nome)
	}
	if ricardo.Matricula == nil || *ricardo.Matricula != "000101016701" {
		t.Errorf("ricardo.Matricula = %v, want 000101016701", ricardo.Matricula)
	}
	if ricardo.Salario == nil || *ricardo.Salario != 6480.00 {
		t.Errorf("ricardo.Salario = %v, want 6480.00", ricardo.Salario)
	}
	if ricardo.Admissao == nil || *ricardo.Admissao != "2026-05-18" {
		t.Errorf("ricardo.Admissao = %v, want 2026-05-18", ricardo.Admissao)
	}

	fulano := result.Linhas[1]
	if fulano.Matricula != nil {
		t.Errorf("fulano.Matricula = %v, want nil (dash)", *fulano.Matricula)
	}
	if fulano.UltimoAumento == nil || *fulano.UltimoAumento != "2026-03-02" {
		t.Errorf("fulano.UltimoAumento = %v, want 2026-03-02 (excel serial 46083)", fulano.UltimoAumento)
	}
	if fulano.TimeSquad != "DEVOPS NOVA" {
		t.Errorf("fulano.TimeSquad = %q", fulano.TimeSquad)
	}
}

func floatPtr(f float64) *float64 { return &f }
func strPtr(s string) *string     { return &s }
