package service

import (
	"strings"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/google/uuid"
)

func MatchLinhas(linhas []domain.ImportPlanilhaLinha, ignorados []domain.ImportIgnorado, membros []domain.Membro, equipes []domain.Equipe) *domain.ImportMatchResult {
	membroByNome := make(map[string]domain.Membro, len(membros))
	for _, m := range membros {
		membroByNome[NormalizeNome(m.Nome)] = m
	}
	equipeByNome := make(map[string]domain.Equipe, len(equipes))
	for _, e := range equipes {
		equipeByNome[NormalizeNome(e.Nome)] = e
	}

	result := &domain.ImportMatchResult{
		Matched:           []domain.ImportMatched{},
		UnmatchedMembros:  []domain.ImportUnmatchedMembro{},
		UnmatchedEquipes:  []domain.ImportUnmatchedEquipe{},
		UnmatchedGestores: []domain.ImportUnmatchedGestor{},
		Ignorados:         ignorados,
	}
	if result.Ignorados == nil {
		result.Ignorados = []domain.ImportIgnorado{}
	}

	unmatchedEquipeLinhas := map[string][]int{}
	var unmatchedEquipeOrder []string
	unmatchedGestorLinhas := map[string][]int{}
	var unmatchedGestorOrder []string

	for _, linha := range linhas {
		cargo := ExtractCargoNivel(linha.Funcao)

		var gestorID *uuid.UUID
		gestorNome := strings.TrimSpace(linha.Gestao)
		if gestorNome != "" {
			if g, ok := membroByNome[NormalizeNome(gestorNome)]; ok {
				id := g.ID
				gestorID = &id
			} else {
				if _, seen := unmatchedGestorLinhas[gestorNome]; !seen {
					unmatchedGestorOrder = append(unmatchedGestorOrder, gestorNome)
				}
				unmatchedGestorLinhas[gestorNome] = append(unmatchedGestorLinhas[gestorNome], linha.Linha)
			}
		}

		dados := domain.ImportDados{
			Cargo:         cargo,
			Matricula:     linha.Matricula,
			Salario:       linha.Salario,
			DataAdmissao:  linha.Admissao,
			UltimoAumento: linha.UltimoAumento,
			GestorNome:    gestorNome,
			GestorID:      gestorID,
		}

		membro, ok := membroByNome[NormalizeNome(linha.Nome)]
		if !ok {
			result.UnmatchedMembros = append(result.UnmatchedMembros, domain.ImportUnmatchedMembro{
				Linha:        linha.Linha,
				NomePlanilha: linha.Nome,
				Dados:        dados,
			})
			continue
		}

		var equipeID *uuid.UUID
		equipeNome := ""
		timeSquad := strings.TrimSpace(linha.TimeSquad)
		if timeSquad != "" {
			if eq, ok := equipeByNome[NormalizeNome(timeSquad)]; ok {
				id := eq.ID
				equipeID = &id
				equipeNome = eq.Nome
			} else {
				if _, seen := unmatchedEquipeLinhas[timeSquad]; !seen {
					unmatchedEquipeOrder = append(unmatchedEquipeOrder, timeSquad)
				}
				unmatchedEquipeLinhas[timeSquad] = append(unmatchedEquipeLinhas[timeSquad], linha.Linha)
			}
		}

		result.Matched = append(result.Matched, domain.ImportMatched{
			Linha:        linha.Linha,
			NomePlanilha: linha.Nome,
			MembroID:     membro.ID,
			MembroNome:   membro.Nome,
			EquipeID:     equipeID,
			EquipeNome:   equipeNome,
			Dados:        dados,
			Changes:      computeChanges(membro, dados),
		})
	}

	for _, nome := range unmatchedEquipeOrder {
		result.UnmatchedEquipes = append(result.UnmatchedEquipes, domain.ImportUnmatchedEquipe{NomePlanilha: nome, Linhas: unmatchedEquipeLinhas[nome]})
	}
	for _, nome := range unmatchedGestorOrder {
		result.UnmatchedGestores = append(result.UnmatchedGestores, domain.ImportUnmatchedGestor{NomePlanilha: nome, Linhas: unmatchedGestorLinhas[nome]})
	}

	return result
}

func computeChanges(m domain.Membro, dados domain.ImportDados) []string {
	changes := []string{}
	if dados.Salario != nil && (m.Salario == nil || *m.Salario != *dados.Salario) {
		changes = append(changes, "salario")
	}
	if dados.DataAdmissao != nil {
		cur := ""
		if m.DataAdmissao != nil {
			cur = m.DataAdmissao.Format("2006-01-02")
		}
		if cur != *dados.DataAdmissao {
			changes = append(changes, "data_admissao")
		}
	}
	if dados.Cargo != nil && (m.Cargo == nil || *m.Cargo != *dados.Cargo) {
		changes = append(changes, "cargo")
	}
	if dados.Matricula != nil && (m.Matricula == nil || *m.Matricula != *dados.Matricula) {
		changes = append(changes, "matricula")
	}
	if dados.UltimoAumento != nil {
		cur := ""
		if m.UltimoAumento != nil {
			cur = m.UltimoAumento.Format("2006-01-02")
		}
		if cur != *dados.UltimoAumento {
			changes = append(changes, "ultimo_aumento")
		}
	}
	return changes
}
