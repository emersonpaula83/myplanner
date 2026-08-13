package service

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
)

func ParseSalarioBR(s string) (*float64, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return nil, nil
	}
	s = strings.ReplaceAll(s, "R$", "")
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, fmt.Errorf("salário inválido %q: %w", s, err)
	}
	return &v, nil
}

func ParseDataPlanilha(s string) (*string, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return nil, nil
	}
	if t, err := time.Parse("02/01/2006", s); err == nil {
		formatted := t.Format("2006-01-02")
		return &formatted, nil
	}
	if serial, err := strconv.Atoi(s); err == nil {
		epoch := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
		t := epoch.AddDate(0, 0, serial)
		formatted := t.Format("2006-01-02")
		return &formatted, nil
	}
	return nil, fmt.Errorf("data inválida %q", s)
}

func NormalizeNome(s string) string {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(s)))
	return strings.Join(fields, " ")
}

type cargoRule struct {
	label string
	slug  string
	exact bool
}

var cargoNivelRules = []cargoRule{
	{"ANALISTA I", domain.CargoAnalistaI, true},
	{"ANALISTA II", domain.CargoAnalistaII, true},
	{"ANALISTA III", domain.CargoAnalistaIII, false},
	{"ESPECIALISTA I", domain.CargoEspecialistaI, true},
	{"ESPECIALISTA II", domain.CargoEspecialistaII, false},
	{"MASTER", domain.CargoMaster, false},
	{"COORDENADOR", domain.CargoCoordenadorDesenvolvimento, false},
	{"LÍDER", domain.CargoLiderTecnico, false},
	{"LIDER", domain.CargoLiderTecnico, false},
	{"TÉCNICO", domain.CargoAnalistaI, false},
	{"TECNICO", domain.CargoAnalistaI, false},
}

func ExtractCargoNivel(funcao string) *string {
	upper := strings.ToUpper(strings.TrimSpace(funcao))
	if upper == "" {
		return nil
	}
	for _, rule := range cargoNivelRules {
		matched := false
		if rule.exact {
			matched = strings.HasSuffix(upper, rule.label) || strings.Contains(upper, rule.label+" ")
		} else {
			matched = strings.Contains(upper, rule.label)
		}
		if matched {
			slug := rule.slug
			return &slug
		}
	}
	return nil
}

var csvHeaderAliases = map[string]string{
	"NOME":            "nome",
	"GESTAO":          "gestao",
	"GESTÃO":          "gestao",
	"TIME / SQUAD":    "time_squad",
	"TIME/SQUAD":      "time_squad",
	"FUNCAO":          "funcao",
	"FUNÇÃO":          "funcao",
	"MATRICULA":       "matricula",
	"MATRÍCULA":       "matricula",
	"ADMISSAO":        "admissao",
	"ADMISSÃO":        "admissao",
	"SALARIO":         "salario",
	"SALÁRIO":         "salario",
	"ULTIMO AUMENTO":  "ultimo_aumento",
	"ÚLTIMO AUMENTO":  "ultimo_aumento",
}

func ParseCSVPlanilha(csvContent string) (*domain.ImportParseResult, error) {
	reader := csv.NewReader(strings.NewReader(csvContent))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading csv: %w", err)
	}

	headerIdx := -1
	colIdx := map[string]int{}
	for i, rec := range records {
		if len(rec) == 0 {
			continue
		}
		if strings.ToUpper(strings.TrimSpace(rec[0])) == "NOME" {
			headerIdx = i
			for j, cell := range rec {
				key := strings.ToUpper(strings.TrimSpace(cell))
				if norm, ok := csvHeaderAliases[key]; ok {
					colIdx[norm] = j
				}
			}
			break
		}
	}
	if headerIdx == -1 {
		return nil, fmt.Errorf("cabeçalho não encontrado (coluna 'Nome' não localizada)")
	}

	result := &domain.ImportParseResult{
		Linhas:    []domain.ImportPlanilhaLinha{},
		Ignorados: []domain.ImportIgnorado{},
	}

	linha := 0
	for _, rec := range records[headerIdx+1:] {
		if isBlankRow(rec) {
			continue
		}
		linha++

		nome := strings.TrimSpace(cellAt(rec, colIdx, "nome"))
		if nome == "" {
			linha--
			continue
		}

		if strings.Contains(strings.ToUpper(nome), "SUB") {
			result.Ignorados = append(result.Ignorados, domain.ImportIgnorado{Linha: linha, Nome: nome, Motivo: "SUB"})
			continue
		}
		if _, err := strconv.Atoi(nome); err == nil {
			result.Ignorados = append(result.Ignorados, domain.ImportIgnorado{Linha: linha, Nome: nome, Motivo: "total"})
			continue
		}

		matriculaRaw := strings.TrimSpace(cellAt(rec, colIdx, "matricula"))
		var matricula *string
		if matriculaRaw != "" && matriculaRaw != "-" {
			matricula = &matriculaRaw
		}

		salario, err := ParseSalarioBR(cellAt(rec, colIdx, "salario"))
		if err != nil {
			return nil, fmt.Errorf("linha %d: %w", linha, err)
		}
		admissao, err := ParseDataPlanilha(cellAt(rec, colIdx, "admissao"))
		if err != nil {
			return nil, fmt.Errorf("linha %d: %w", linha, err)
		}
		ultimoAumento, err := ParseDataPlanilha(cellAt(rec, colIdx, "ultimo_aumento"))
		if err != nil {
			return nil, fmt.Errorf("linha %d: %w", linha, err)
		}

		result.Linhas = append(result.Linhas, domain.ImportPlanilhaLinha{
			Linha:         linha,
			Nome:          nome,
			Gestao:        strings.TrimSpace(cellAt(rec, colIdx, "gestao")),
			TimeSquad:     strings.TrimSpace(cellAt(rec, colIdx, "time_squad")),
			Funcao:        strings.TrimSpace(cellAt(rec, colIdx, "funcao")),
			Matricula:     matricula,
			Admissao:      admissao,
			Salario:       salario,
			UltimoAumento: ultimoAumento,
		})
	}

	return result, nil
}

func cellAt(rec []string, colIdx map[string]int, key string) string {
	idx, ok := colIdx[key]
	if !ok || idx >= len(rec) {
		return ""
	}
	return rec[idx]
}

func isBlankRow(rec []string) bool {
	for _, c := range rec {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}
