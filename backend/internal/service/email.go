package service

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"math"
	"net/smtp"
	"strings"

	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"go.uber.org/zap"
)

type EmailProvider struct {
	configRepo *repository.ConfigRepository
	logger     *zap.Logger
}

func NewEmailProvider(configRepo *repository.ConfigRepository, logger *zap.Logger) *EmailProvider {
	return &EmailProvider{configRepo: configRepo, logger: logger}
}

func (p *EmailProvider) Send(ctx context.Context, to string, subject string, htmlBody string) error {
	host, err := p.configRepo.GetConfig(ctx, "smtp_host")
	if err != nil {
		return fmt.Errorf("smtp_host not configured: %w", err)
	}
	port, err := p.configRepo.GetConfig(ctx, "smtp_port")
	if err != nil {
		return fmt.Errorf("smtp_port not configured: %w", err)
	}
	user, err := p.configRepo.GetConfig(ctx, "smtp_user")
	if err != nil {
		return fmt.Errorf("smtp_user not configured: %w", err)
	}
	password, err := p.configRepo.GetConfig(ctx, "smtp_password")
	if err != nil {
		return fmt.Errorf("smtp_password not configured: %w", err)
	}
	from, err := p.configRepo.GetConfig(ctx, "smtp_from")
	if err != nil {
		from = user
	}

	to = sanitizeHeaderValue(to)
	subject = sanitizeHeaderValue(subject)
	from = sanitizeHeaderValue(from)

	msg := "From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=\"UTF-8\"\r\n" +
		"\r\n" + htmlBody

	auth := smtp.PlainAuth("", user, password, host)
	addr := host + ":" + port
	return smtp.SendMail(addr, auth, user, []string{to}, []byte(msg))
}

// sanitizeHeaderValue strips CR/LF characters to prevent SMTP header injection.
func sanitizeHeaderValue(v string) string {
	v = strings.ReplaceAll(v, "\r", "")
	v = strings.ReplaceAll(v, "\n", "")
	return v
}

type emailTemplateData struct {
	SprintNome        string
	Stats             ReviewStats
	PctConcluidas     int
	PctPlanConcluidas int
	PctBugs           int
	PctMelhorias      int
	PctOutros         int
	GraficoProdutos   []ReviewGraficoProduto
	GraficoCategorias []ReviewGraficoCategoria
	Tarefas           []ReviewTarefa
	Analise           string
	HasAnalise        bool
	ProdutosSVG       template.HTML
	CategoriasSVG     template.HTML
	PlanejamentoSVG   template.HTML
}

func (p *EmailProvider) RenderReviewEmail(data *ReviewData, analise string, sprintNome string) (string, error) {
	total := data.Stats.Total
	if total == 0 {
		total = 1
	}
	planTotal := data.Stats.PlanejadasTotal
	if planTotal == 0 {
		planTotal = 1
	}

	td := emailTemplateData{
		SprintNome:        sprintNome,
		Stats:             data.Stats,
		PctConcluidas:     data.Stats.Concluidas * 100 / total,
		PctPlanConcluidas: data.Stats.PlanejadasConcl * 100 / planTotal,
		PctBugs:           data.Stats.BugsIncidentes * 100 / total,
		PctMelhorias:      data.Stats.MelhoriasInovacoes * 100 / total,
		PctOutros:         data.Stats.Outros * 100 / total,
		GraficoProdutos:   data.GraficoProdutos,
		GraficoCategorias: data.GraficoCategorias,
		Tarefas:           data.Tarefas,
		Analise:           analise,
		HasAnalise:        analise != "",
		ProdutosSVG:       template.HTML(generatePieChartSVG(buildProdutosSlices(data.GraficoProdutos), "Produtos")),
		CategoriasSVG:     template.HTML(generatePieChartSVG(buildCategoriasSlices(data.GraficoCategorias), "Categorias")),
		PlanejamentoSVG:   template.HTML(generatePlanejamentoSVG(data.GraficoPlanejamento)),
	}

	tmpl, err := template.New("review-email").Parse(reviewEmailTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing email template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, td); err != nil {
		return "", fmt.Errorf("executing email template: %w", err)
	}
	return buf.String(), nil
}

type pieSlice struct {
	Label string
	Value int
	Color string
}

var pieColors = []string{"#2563eb", "#16a34a", "#eab308", "#ef4444", "#8b5cf6", "#f97316", "#06b6d4", "#ec4899"}

func buildProdutosSlices(prods []ReviewGraficoProduto) []pieSlice {
	var slices []pieSlice
	for i, p := range prods {
		slices = append(slices, pieSlice{Label: p.Produto, Value: p.Total, Color: pieColors[i%len(pieColors)]})
	}
	return slices
}

func buildCategoriasSlices(cats []ReviewGraficoCategoria) []pieSlice {
	var slices []pieSlice
	for i, c := range cats {
		slices = append(slices, pieSlice{Label: c.Categoria, Value: c.Total, Color: pieColors[i%len(pieColors)]})
	}
	return slices
}

func generatePieChartSVG(slices []pieSlice, title string) string {
	if len(slices) == 0 {
		return ""
	}
	total := 0
	for _, s := range slices {
		total += s.Value
	}
	if total == 0 {
		return ""
	}

	nonZeroSlices := 0
	for _, s := range slices {
		if s.Value > 0 {
			nonZeroSlices++
		}
	}

	cx, cy, r := 80.0, 80.0, 70.0
	var svg strings.Builder
	svg.WriteString(fmt.Sprintf(`<svg width="180" height="200" xmlns="http://www.w3.org/2000/svg">
<text x="90" y="16" text-anchor="middle" font-size="12" font-weight="bold" fill="#334155">%s</text>`, title))

	startAngle := -90.0
	for _, s := range slices {
		pct := float64(s.Value) / float64(total)
		angle := pct * 360.0
		endAngle := startAngle + angle
		largeArc := 0
		if angle > 180 {
			largeArc = 1
		}

		x1 := cx + r*math.Cos(startAngle*math.Pi/180)
		y1 := cy + r*math.Sin(startAngle*math.Pi/180) + 20
		x2 := cx + r*math.Cos(endAngle*math.Pi/180)
		y2 := cy + r*math.Sin(endAngle*math.Pi/180) + 20

		if nonZeroSlices == 1 {
			if s.Value > 0 {
				svg.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s"/>`, cx, cy+20, r, s.Color))
			}
		} else {
			svg.WriteString(fmt.Sprintf(`<path d="M%.1f,%.1f L%.1f,%.1f A%.1f,%.1f 0 %d,1 %.1f,%.1f Z" fill="%s"/>`,
				cx, cy+20, x1, y1, r, r, largeArc, x2, y2, s.Color))
		}
		startAngle = endAngle
	}

	y := 175.0
	for _, s := range slices {
		if y > 195 {
			break
		}
		svg.WriteString(fmt.Sprintf(`<rect x="2" y="%.0f" width="8" height="8" fill="%s"/>`, y, s.Color))
		svg.WriteString(fmt.Sprintf(`<text x="14" y="%.0f" font-size="9" fill="#475569">%s (%d)</text>`, y+8, template.HTMLEscapeString(s.Label), s.Value))
		y += 12
	}

	svg.WriteString(`</svg>`)
	return svg.String()
}

func generatePlanejamentoSVG(plan ReviewGraficoPlanejamento) string {
	total := plan.Planejadas + plan.NaoPlanejadas
	if total == 0 {
		return ""
	}
	slices := []pieSlice{
		{Label: "Planejadas", Value: plan.Planejadas, Color: "#2563eb"},
		{Label: "Não Plan. (Bugs)", Value: plan.NaoPlanejadasBugs, Color: "#ef4444"},
		{Label: "Não Plan. (Outras)", Value: plan.NaoPlanejadasOutras, Color: "#f97316"},
	}
	return generatePieChartSVG(slices, "Planejamento")
}

const reviewEmailTemplate = `<table width="600" cellpadding="0" cellspacing="0" style="margin:0 auto;font-family:Arial,Helvetica,sans-serif;border:1px solid #e2e8f0">
<tr><td style="background:#2563eb;color:#fff;padding:24px;font-size:20px;font-weight:bold">
📊 Sprint Review — {{.SprintNome}}
</td></tr>
<tr><td style="padding:16px">
<table width="100%" cellpadding="0" cellspacing="8"><tr>
<td width="25%" style="background:#f0fdf4;padding:12px;text-align:center;border-radius:8px">
<div style="font-size:24px;font-weight:bold;color:#16a34a">{{.Stats.Concluidas}}/{{.Stats.Total}}</div>
<div style="font-size:11px;color:#666">Concluídas ({{.PctConcluidas}}%)</div>
</td>
<td width="25%" style="background:#eff6ff;padding:12px;text-align:center;border-radius:8px">
<div style="font-size:24px;font-weight:bold;color:#2563eb">{{.Stats.EmAndamento}}</div>
<div style="font-size:11px;color:#666">Em Andamento</div>
</td>
<td width="25%" style="background:#fef2f2;padding:12px;text-align:center;border-radius:8px">
<div style="font-size:24px;font-weight:bold;color:#ef4444">{{.Stats.NaoIniciadas}}</div>
<div style="font-size:11px;color:#666">Não Iniciadas</div>
</td>
<td width="25%" style="background:#fefce8;padding:12px;text-align:center;border-radius:8px">
<div style="font-size:24px;font-weight:bold;color:#ca8a04">{{.PctPlanConcluidas}}%</div>
<div style="font-size:11px;color:#666">Planejamento</div>
</td>
</tr></table>
</td></tr>
<tr><td style="padding:16px">
<table width="100%" cellpadding="0" cellspacing="0"><tr>
<td width="33%" valign="top">{{.ProdutosSVG}}</td>
<td width="33%" valign="top">{{.CategoriasSVG}}</td>
<td width="33%" valign="top">{{.PlanejamentoSVG}}</td>
</tr></table>
</td></tr>
<tr><td style="padding:16px">
<table width="100%" cellpadding="6" cellspacing="0" style="border-collapse:collapse;font-size:12px">
<tr style="background:#f1f5f9">
<th style="text-align:left;padding:8px;border-bottom:2px solid #cbd5e1">Ticket</th>
<th style="text-align:left;padding:8px;border-bottom:2px solid #cbd5e1">Resumo</th>
<th style="text-align:left;padding:8px;border-bottom:2px solid #cbd5e1">Tipo</th>
<th style="text-align:left;padding:8px;border-bottom:2px solid #cbd5e1">Status</th>
</tr>
{{range .Tarefas}}
<tr style="border-bottom:1px solid #e2e8f0">
<td style="padding:6px 8px;font-family:monospace;font-size:11px">{{.NumeroTicket}}</td>
<td style="padding:6px 8px">{{.Resumo}}</td>
<td style="padding:6px 8px">{{.TipoDemanda}}</td>
<td style="padding:6px 8px;font-weight:bold">{{.Status}}</td>
</tr>
{{end}}
</table>
</td></tr>
{{if .HasAnalise}}
<tr><td style="padding:16px">
<table width="100%" cellpadding="0" cellspacing="0"><tr>
<td style="background:#f8fafc;padding:16px;border-radius:8px;border:1px solid #e2e8f0">
<div style="font-weight:bold;margin-bottom:8px;font-size:14px">🤖 Análise da IA</div>
<div style="font-size:13px;line-height:1.6;white-space:pre-wrap">{{.Analise}}</div>
</td>
</tr></table>
</td></tr>
{{end}}
<tr><td style="padding:16px;text-align:center;color:#94a3b8;font-size:11px;border-top:1px solid #e2e8f0">
Gerado por MyPlanner
</td></tr>
</table>`
