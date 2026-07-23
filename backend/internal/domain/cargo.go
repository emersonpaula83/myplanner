package domain

import "slices"

const (
	CargoCoordenadorDesenvolvimento = "coordenador_desenvolvimento"
	CargoPOProduto                  = "po_produto"
	CargoGerenteTecnologia          = "gerente_tecnologia"
	CargoGerenteExecutivo           = "gerente_executivo"
	CargoScrumMaster                = "scrum_master"
	CargoAgileMaster                = "agile_master"
	CargoDesenvolvedor              = "desenvolvedor"
)

var CargosValidos = []string{
	CargoCoordenadorDesenvolvimento,
	CargoPOProduto,
	CargoGerenteTecnologia,
	CargoGerenteExecutivo,
	CargoScrumMaster,
	CargoAgileMaster,
	CargoDesenvolvedor,
}

var CargoLabels = map[string]string{
	CargoCoordenadorDesenvolvimento: "Coordenador de Desenvolvimento",
	CargoPOProduto:                  "P.O. Produto",
	CargoGerenteTecnologia:          "Gerente de Tecnologia",
	CargoGerenteExecutivo:           "Gerente Executivo",
	CargoScrumMaster:                "Scrum Master",
	CargoAgileMaster:                "Agile Master",
	CargoDesenvolvedor:              "Desenvolvedor",
}

func IsCargoValido(cargo string) bool {
	return slices.Contains(CargosValidos, cargo)
}
