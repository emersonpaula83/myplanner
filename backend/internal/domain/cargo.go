package domain

import "slices"

const (
	CargoAnalistaI                  = "analista_i"
	CargoAnalistaII                 = "analista_ii"
	CargoAnalistaIII                = "analista_iii"
	CargoEspecialistaI              = "especialista_i"
	CargoEspecialistaII             = "especialista_ii"
	CargoMaster                     = "master"
	CargoCoordenadorDesenvolvimento = "coordenador_desenvolvimento"
	CargoLiderTecnico               = "lider_tecnico"
)

var CargosValidos = []string{
	CargoAnalistaI,
	CargoAnalistaII,
	CargoAnalistaIII,
	CargoEspecialistaI,
	CargoEspecialistaII,
	CargoMaster,
	CargoCoordenadorDesenvolvimento,
	CargoLiderTecnico,
}

var CargoLabels = map[string]string{
	CargoAnalistaI:                  "Analista I",
	CargoAnalistaII:                 "Analista II",
	CargoAnalistaIII:                "Analista III",
	CargoEspecialistaI:              "Especialista I",
	CargoEspecialistaII:             "Especialista II",
	CargoMaster:                     "Master",
	CargoCoordenadorDesenvolvimento: "Coord. Dev",
	CargoLiderTecnico:               "Líder Técnico",
}

func IsCargoValido(cargo string) bool {
	return slices.Contains(CargosValidos, cargo)
}

var PromocoesValidas = map[string][]string{
	CargoAnalistaI:                  {CargoAnalistaII},
	CargoAnalistaII:                 {CargoAnalistaIII},
	CargoAnalistaIII:                {CargoEspecialistaI, CargoCoordenadorDesenvolvimento},
	CargoEspecialistaI:              {CargoEspecialistaII, CargoCoordenadorDesenvolvimento, CargoLiderTecnico},
	CargoEspecialistaII:             {CargoMaster, CargoLiderTecnico},
	CargoCoordenadorDesenvolvimento: {CargoLiderTecnico},
}

func IsPromocaoValida(cargoAtual, cargoNovo string) bool {
	validas, ok := PromocoesValidas[cargoAtual]
	if !ok {
		return false
	}
	return slices.Contains(validas, cargoNovo)
}
