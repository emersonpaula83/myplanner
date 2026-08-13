package service

import (
	"testing"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/google/uuid"
)

func TestMatchLinhas_MatchByNameCaseInsensitive(t *testing.T) {
	membroID := uuid.New()
	membros := []domain.Membro{{ID: membroID, Nome: "Ricardo Kazuo Diniz Nozaki"}}
	equipes := []domain.Equipe{}
	salario := 6480.00
	linhas := []domain.ImportPlanilhaLinha{
		{Linha: 1, Nome: "RICARDO KAZUO DINIZ NOZAKI", Salario: &salario},
	}

	result := MatchLinhas(linhas, nil, membros, equipes)

	if len(result.Matched) != 1 {
		t.Fatalf("got %d matched, want 1", len(result.Matched))
	}
	if result.Matched[0].MembroID != membroID {
		t.Errorf("MembroID = %v, want %v", result.Matched[0].MembroID, membroID)
	}
	if len(result.UnmatchedMembros) != 0 {
		t.Errorf("got %d unmatched membros, want 0", len(result.UnmatchedMembros))
	}
}

func TestMatchLinhas_UnmatchedMembro(t *testing.T) {
	membros := []domain.Membro{{ID: uuid.New(), Nome: "Outra Pessoa"}}
	linhas := []domain.ImportPlanilhaLinha{{Linha: 5, Nome: "Fulano De Tal"}}

	result := MatchLinhas(linhas, nil, membros, nil)

	if len(result.Matched) != 0 {
		t.Fatalf("got %d matched, want 0", len(result.Matched))
	}
	if len(result.UnmatchedMembros) != 1 || result.UnmatchedMembros[0].Linha != 5 {
		t.Fatalf("unexpected unmatched membros: %+v", result.UnmatchedMembros)
	}
}

func TestMatchLinhas_UnmatchedEquipeGroupsLinhas(t *testing.T) {
	m1, m2 := uuid.New(), uuid.New()
	membros := []domain.Membro{
		{ID: m1, Nome: "Pessoa Um"},
		{ID: m2, Nome: "Pessoa Dois"},
	}
	linhas := []domain.ImportPlanilhaLinha{
		{Linha: 1, Nome: "Pessoa Um", TimeSquad: "DEVOPS NOVA"},
		{Linha: 2, Nome: "Pessoa Dois", TimeSquad: "DEVOPS NOVA"},
	}

	result := MatchLinhas(linhas, nil, membros, nil)

	if len(result.Matched) != 2 {
		t.Fatalf("got %d matched, want 2", len(result.Matched))
	}
	if result.Matched[0].EquipeID != nil {
		t.Errorf("expected nil EquipeID for unmatched squad")
	}
	if len(result.UnmatchedEquipes) != 1 {
		t.Fatalf("got %d unmatched equipes, want 1", len(result.UnmatchedEquipes))
	}
	if got := result.UnmatchedEquipes[0].Linhas; len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("UnmatchedEquipes[0].Linhas = %v, want [1 2]", got)
	}
}

func TestMatchLinhas_UnmatchedGestor(t *testing.T) {
	m1 := uuid.New()
	membros := []domain.Membro{{ID: m1, Nome: "Pessoa Um"}}
	linhas := []domain.ImportPlanilhaLinha{
		{Linha: 3, Nome: "Pessoa Um", Gestao: "Novo Gestor"},
	}

	result := MatchLinhas(linhas, nil, membros, nil)

	if len(result.UnmatchedGestores) != 1 || result.UnmatchedGestores[0].NomePlanilha != "Novo Gestor" {
		t.Fatalf("unexpected unmatched gestores: %+v", result.UnmatchedGestores)
	}
	if result.Matched[0].Dados.GestorID != nil {
		t.Errorf("expected nil GestorID for unmatched gestor")
	}
}

func TestComputeChanges(t *testing.T) {
	salarioAtual := 5000.0
	m := domain.Membro{Salario: &salarioAtual}
	novoSalario := 6000.0
	dados := domain.ImportDados{Salario: &novoSalario}

	changes := computeChanges(m, dados)

	if len(changes) != 1 || changes[0] != "salario" {
		t.Errorf("changes = %v, want [salario]", changes)
	}
}

func TestComputeChanges_NoChange(t *testing.T) {
	salarioAtual := 5000.0
	m := domain.Membro{Salario: &salarioAtual}
	dados := domain.ImportDados{Salario: &salarioAtual}

	changes := computeChanges(m, dados)

	if len(changes) != 0 {
		t.Errorf("changes = %v, want []", changes)
	}
}

func TestExtractSheetsIDAndGid(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantID  string
		wantGid string
		wantErr bool
	}{
		{"edit with hash gid", "https://docs.google.com/spreadsheets/d/1AbC-xyz_123/edit#gid=456", "1AbC-xyz_123", "456", false},
		{"edit no gid defaults to 0", "https://docs.google.com/spreadsheets/d/1AbC-xyz_123/edit", "1AbC-xyz_123", "0", false},
		{"query gid", "https://docs.google.com/spreadsheets/d/1AbC-xyz_123/edit?gid=789#gid=789", "1AbC-xyz_123", "789", false},
		{"invalid url", "https://example.com/not-a-sheet", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, gid, err := ExtractSheetsIDAndGid(tt.url)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if id != tt.wantID || gid != tt.wantGid {
				t.Errorf("got id=%q gid=%q, want id=%q gid=%q", id, gid, tt.wantID, tt.wantGid)
			}
		})
	}
}
