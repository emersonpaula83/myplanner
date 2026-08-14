package repository

import (
	"testing"

	"github.com/google/uuid"
)

func linhaProduto(bucket string, id uuid.UUID, nome string, cards int, horas float64) produtoLinha {
	return produtoLinha{Bucket: bucket, ProdutoID: &id, Nome: nome, Cards: cards, Horas: horas}
}

func TestMontarProdutosPorBucketOrdenaECortaNoTopo(t *testing.T) {
	linhas := []produtoLinha{}
	// 10 produtos com horas crescentes: o corte deve ficar com os maiores.
	for i := 1; i <= 10; i++ {
		linhas = append(linhas, linhaProduto(BucketMelhorias, uuid.New(), "P"+string(rune('A'+i-1)), i, float64(i)*10))
	}

	porBucket := montarProdutosPorBucket(linhas)
	mel := porBucket[BucketMelhorias]

	if len(mel.Produtos) != esforcoTopProdutos {
		t.Fatalf("esperava %d produtos no topo, veio %d", esforcoTopProdutos, len(mel.Produtos))
	}
	if mel.Produtos[0].Horas != 100 || mel.Produtos[1].Horas != 90 {
		t.Errorf("topo fora de ordem: %v", mel.Produtos)
	}
	if mel.TotalProdutos != 10 {
		t.Errorf("TotalProdutos = %d, esperava 10", mel.TotalProdutos)
	}
	// Totais cobrem todos os produtos, não só os exibidos — é o que permite ao
	// frontend montar a linha "Outros".
	if mel.HorasSomadas != 550 {
		t.Errorf("HorasSomadas = %v, esperava 550", mel.HorasSomadas)
	}
}

func TestMontarProdutosPorBucketDesempataPorNome(t *testing.T) {
	linhas := []produtoLinha{
		linhaProduto(BucketManutencao, uuid.New(), "Zebra", 1, 10),
		linhaProduto(BucketManutencao, uuid.New(), "Abacate", 1, 10),
	}

	prod := montarProdutosPorBucket(linhas)[BucketManutencao].Produtos
	if prod[0].Nome != "Abacate" {
		t.Errorf("empate deveria ordenar por nome, veio %q primeiro", prod[0].Nome)
	}
}

func TestMontarProdutosPorBucketSeparaCardsSemProduto(t *testing.T) {
	linhas := []produtoLinha{
		linhaProduto(BucketOutros, uuid.New(), "Nostromo", 3, 30),
		{Bucket: BucketOutros, ProdutoID: nil, Cards: 7, Horas: 70},
	}

	out := montarProdutosPorBucket(linhas)[BucketOutros]
	if len(out.Produtos) != 1 {
		t.Fatalf("linha sem produto virou produto: %v", out.Produtos)
	}
	if out.SemProduto.Cards != 7 || out.SemProduto.Horas != 70 {
		t.Errorf("SemProduto = %+v, esperava 7 cards / 70h", out.SemProduto)
	}
	// Cards sem produto não entram nos totais somados: eles não estão no gráfico.
	if out.HorasSomadas != 30 || out.TotalProdutos != 1 {
		t.Errorf("totais somados contaminados por cards sem produto: %+v", out)
	}
}

func TestMontarProdutosPorBucketSempreTrazOsTresBaldes(t *testing.T) {
	porBucket := montarProdutosPorBucket(nil)

	for _, def := range bucketsOrdenados {
		b, ok := porBucket[def.Chave]
		if !ok {
			t.Fatalf("balde %q ausente — frontend quebraria ao clicar na fatia", def.Chave)
		}
		if b.Produtos == nil {
			t.Errorf("balde %q com Produtos nil; esperava lista vazia para virar [] no JSON", def.Chave)
		}
	}
}
