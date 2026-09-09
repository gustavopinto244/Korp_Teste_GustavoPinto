package ia

// Integração real com a API da Anthropic (Claude), usada quando
// IA_PROVIDER=claude. É a única implementação de InterpretadorDeTexto que
// faz chamada de rede; a alternativa (IA_PROVIDER=mock, o padrão) é a
// heurística local de mock_interpretador.go.
//
// Duas decisões governam este arquivo:
//
//   - A saída é estruturada por tool use com schema estrito, não por texto
//     livre. O modelo preenche os argumentos de uma ferramenta cujo JSON
//     Schema descreve exatamente ResultadoInterpretacao, então não há
//     parsing de prosa nem prompt pedindo "responda em JSON".
//   - O prompt pede que o modelo não invente produtos, mas quem garante
//     isso é o código: conciliarComCatalogo descarta qualquer código que
//     não esteja no catálogo real vindo do estoque e reescreve a descrição
//     a partir dele. O modelo escolhe; o catálogo decide.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// ModeloPadrao é o modelo usado quando IA_MODEL não é configurada.
const ModeloPadrao = "claude-opus-5"

// nomeFerramenta é o nome da ferramenta que carrega a saída estruturada.
const nomeFerramenta = "registrar_itens"

// maxTokensResposta limita a resposta. A saída é uma lista curta de itens;
// o teto existe para conter uma resposta degenerada, não para caber no
// tamanho normal.
const maxTokensResposta = 4096

const instrucaoSistema = `Você converte pedidos escritos em português coloquial em itens de nota fiscal.

Regras:
- Só pode usar produtos do catálogo informado na mensagem do usuário. Nunca invente um código.
- Cada item precisa de uma quantidade inteira maior que zero. Se o texto não disser a quantidade, o trecho é não reconhecido.
- Trecho que não casar com nenhum produto do catálogo vai para itensNaoReconhecidos, com o motivo em português.
- Um produto citado duas vezes no texto vira um item só, com as quantidades somadas.
- Responda exclusivamente chamando a ferramenta ` + nomeFerramenta + `, sem texto antes ou depois.`

// interpretadorClaude implementa InterpretadorDeTexto contra a Messages API
// da Anthropic.
type interpretadorClaude struct {
	cliente anthropic.Client
	modelo  anthropic.Model
}

// NovoInterpretadorClaude constrói o interpretador que fala a Messages API.
// A apiKey vem de IA_API_KEY, o modelo de IA_MODEL (vazio = ModeloPadrao) e
// a baseURL de IA_BASE_URL (vazia = api.anthropic.com). Nenhum desses
// valores é registrado em log.
//
// A baseURL existe porque a Messages API tem implementações compatíveis
// além da própria Anthropic (gateways como o OpenRouter). O SDK oficial
// continua sendo o cliente; só o endereço muda, e com ele o formato do
// identificador de modelo — daí IA_MODEL ser configurável junto.
func NovoInterpretadorClaude(apiKey, modelo, baseURL string) InterpretadorDeTexto {
	if strings.TrimSpace(modelo) == "" {
		modelo = ModeloPadrao
	}

	opcoes := []option.RequestOption{option.WithAPIKey(apiKey)}
	if url := strings.TrimSpace(baseURL); url != "" {
		opcoes = append(opcoes, option.WithBaseURL(url))
	}

	return &interpretadorClaude{
		cliente: anthropic.NewClient(opcoes...),
		modelo:  anthropic.Model(modelo),
	}
}

// esquemaFerramenta descreve, em JSON Schema, exatamente a forma de
// ResultadoInterpretacao. Com Strict o modelo é obrigado a respeitá-lo, o
// que elimina a classe inteira de erros de "o JSON veio diferente".
func esquemaFerramenta() anthropic.ToolInputSchemaParam {
	item := func(propriedades map[string]any, obrigatorios []string) map[string]any {
		return map[string]any{
			"type":                 "array",
			"items":                map[string]any{"type": "object", "properties": propriedades, "required": obrigatorios, "additionalProperties": false},
			"additionalProperties": false,
		}
	}

	return anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"itensSugeridos": item(map[string]any{
				"produtoCodigo": map[string]any{"type": "string", "description": "código exato de um produto do catálogo"},
				"quantidade":    map[string]any{"type": "integer", "minimum": 1},
				"confianca":     map[string]any{"type": "string", "enum": []string{"alta", "media", "baixa"}},
			}, []string{"produtoCodigo", "quantidade", "confianca"}),
			"itensNaoReconhecidos": item(map[string]any{
				"textoOriginal": map[string]any{"type": "string", "description": "o trecho do texto do usuário que não pôde ser casado"},
				"motivo":        map[string]any{"type": "string", "description": "explicação curta, em português, para o usuário"},
			}, []string{"textoOriginal", "motivo"}),
		},
		Required:    []string{"itensSugeridos", "itensNaoReconhecidos"},
		ExtraFields: map[string]any{"additionalProperties": false},
	}
}

func (i *interpretadorClaude) Interpretar(ctx context.Context, texto string, catalogo []CatalogoItem) (ResultadoInterpretacao, error) {
	if len(catalogo) == 0 {
		// Sem catálogo não há o que sugerir, e uma chamada ao modelo só
		// poderia produzir código inventado. Devolve o texto inteiro como
		// não reconhecido, sem gastar requisição.
		return ResultadoInterpretacao{
			ItensSugeridos: []ItemSugerido{},
			ItensNaoReconhecidos: []ItemNaoReconhecido{{
				TextoOriginal: texto,
				Motivo:        "o catálogo de produtos está vazio",
			}},
		}, nil
	}

	ferramenta := anthropic.ToolParam{
		Name:        nomeFerramenta,
		Description: anthropic.String("Registra os itens de nota fiscal identificados no texto do usuário."),
		InputSchema: esquemaFerramenta(),
		Strict:      anthropic.Bool(true),
	}

	resposta, err := i.cliente.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     i.modelo,
		MaxTokens: maxTokensResposta,
		System:    []anthropic.TextBlockParam{{Text: instrucaoSistema}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(montarPrompt(texto, catalogo))),
		},
		Tools: []anthropic.ToolUnionParam{{OfTool: &ferramenta}},
		// A tarefa é curta e mecânica: esforço baixo mantém a latência
		// dentro do que uma tela de cadastro tolera.
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortLow},
	})
	if err != nil {
		return ResultadoInterpretacao{}, fmt.Errorf("chamar a Messages API: %w", resumirErroDeAPI(err))
	}

	bruto, err := extrairArgumentosDaFerramenta(resposta)
	if err != nil {
		return ResultadoInterpretacao{}, err
	}

	return conciliarComCatalogo(bruto, catalogo), nil
}

// limiteErroAPI corta a mensagem de erro que vai para o log. Um endpoint mal
// configurado responde uma página HTML inteira, e o SDK a devolve dentro do
// erro: sem corte, um único 404 despeja milhares de linhas no log do
// serviço e esconde o que importa.
const limiteErroAPI = 300

func resumirErroDeAPI(err error) error {
	runas := []rune(err.Error())
	if len(runas) <= limiteErroAPI {
		return err
	}
	return errors.New(string(runas[:limiteErroAPI]) + "… (mensagem truncada)")
}

// montarPrompt entrega o catálogo e o texto do usuário em blocos
// separados e rotulados, para o modelo não confundir um pedido escrito no
// texto livre com uma instrução.
func montarPrompt(texto string, catalogo []CatalogoItem) string {
	var b strings.Builder
	b.WriteString("<catalogo>\n")
	for _, item := range catalogo {
		fmt.Fprintf(&b, "%s | %s\n", item.Codigo, item.Descricao)
	}
	b.WriteString("</catalogo>\n\n<pedido_do_usuario>\n")
	b.WriteString(texto)
	b.WriteString("\n</pedido_do_usuario>")
	return b.String()
}

// extrairArgumentosDaFerramenta devolve o input da chamada de ferramenta.
// Qualquer outra forma de resposta (texto puro, recusa, corte por limite de
// tokens) é erro: o chamador degrada para IA indisponível.
func extrairArgumentosDaFerramenta(resposta *anthropic.Message) (ResultadoInterpretacao, error) {
	var vazio ResultadoInterpretacao

	if resposta.StopReason == anthropic.StopReasonRefusal {
		return vazio, fmt.Errorf("o modelo recusou a solicitação (%s)", resposta.StopDetails.Category)
	}

	for _, bloco := range resposta.Content {
		uso, ok := bloco.AsAny().(anthropic.ToolUseBlock)
		if !ok || uso.Name != nomeFerramenta {
			continue
		}

		var argumentos ResultadoInterpretacao
		if err := json.Unmarshal([]byte(uso.JSON.Input.Raw()), &argumentos); err != nil {
			return vazio, fmt.Errorf("decodificar argumentos da ferramenta: %w", err)
		}
		return argumentos, nil
	}

	return vazio, fmt.Errorf("o modelo não chamou a ferramenta %s (stop_reason=%s)", nomeFerramenta, resposta.StopReason)
}

// conciliarComCatalogo é a trava que torna a saída do modelo confiável o
// bastante para virar sugestão de nota fiscal:
//
//   - código fora do catálogo vira item não reconhecido, nunca sugestão;
//   - quantidade não positiva vira item não reconhecido;
//   - a descrição exibida vem sempre do catálogo, nunca do modelo;
//   - o mesmo código citado duas vezes é somado em um único item.
//
// Sem ela, um código alucinado chegaria à tela como um produto existente.
func conciliarComCatalogo(bruto ResultadoInterpretacao, catalogo []CatalogoItem) ResultadoInterpretacao {
	porCodigo := make(map[string]CatalogoItem, len(catalogo))
	for _, item := range catalogo {
		porCodigo[item.Codigo] = item
	}

	resultado := ResultadoInterpretacao{
		ItensSugeridos:       []ItemSugerido{},
		ItensNaoReconhecidos: []ItemNaoReconhecido{},
	}

	// ordem de aparição preservada; quantidades do mesmo código somadas
	posicao := make(map[string]int)

	for _, sugerido := range bruto.ItensSugeridos {
		produto, existe := porCodigo[sugerido.ProdutoCodigo]
		if !existe {
			resultado.ItensNaoReconhecidos = append(resultado.ItensNaoReconhecidos, ItemNaoReconhecido{
				TextoOriginal: sugerido.ProdutoCodigo,
				Motivo:        "produto não encontrado no catálogo",
			})
			continue
		}
		if sugerido.Quantidade <= 0 {
			resultado.ItensNaoReconhecidos = append(resultado.ItensNaoReconhecidos, ItemNaoReconhecido{
				TextoOriginal: produto.Descricao,
				Motivo:        "quantidade inválida sugerida para o produto",
			})
			continue
		}
		if produto.ExcedeSaldo(sugerido.Quantidade) {
			resultado.ItensNaoReconhecidos = append(resultado.ItensNaoReconhecidos, ItemNaoReconhecido{
				TextoOriginal: produto.Descricao,
				Motivo: fmt.Sprintf(
					"quantidade sugerida (%d) acima do saldo disponível (%d)",
					sugerido.Quantidade, produto.SaldoDisponivel,
				),
			})
			continue
		}

		if idx, repetido := posicao[produto.Codigo]; repetido {
			somada := resultado.ItensSugeridos[idx].Quantidade + sugerido.Quantidade
			if produto.ExcedeSaldo(somada) {
				resultado.ItensNaoReconhecidos = append(resultado.ItensNaoReconhecidos, ItemNaoReconhecido{
					TextoOriginal: produto.Descricao,
					Motivo: fmt.Sprintf(
						"quantidade acumulada (%d) acima do saldo disponível (%d)",
						somada, produto.SaldoDisponivel,
					),
				})
				continue
			}
			resultado.ItensSugeridos[idx].Quantidade = somada
			continue
		}

		posicao[produto.Codigo] = len(resultado.ItensSugeridos)
		resultado.ItensSugeridos = append(resultado.ItensSugeridos, ItemSugerido{
			ProdutoCodigo:    produto.Codigo,
			ProdutoDescricao: produto.Descricao,
			Quantidade:       sugerido.Quantidade,
			Confianca:        normalizarConfianca(sugerido.Confianca),
		})
	}

	for _, naoReconhecido := range bruto.ItensNaoReconhecidos {
		texto := strings.TrimSpace(naoReconhecido.TextoOriginal)
		if texto == "" {
			continue
		}
		motivo := strings.TrimSpace(naoReconhecido.Motivo)
		if motivo == "" {
			motivo = "não foi possível casar o trecho com um produto do catálogo"
		}
		resultado.ItensNaoReconhecidos = append(resultado.ItensNaoReconhecidos, ItemNaoReconhecido{
			TextoOriginal: texto,
			Motivo:        motivo,
		})
	}

	return resultado
}

// normalizarConfianca protege a tela de um rótulo fora do combinado: o
// schema já restringe os valores a alta/media/baixa, mas a exibição não
// deve depender disso.
func normalizarConfianca(valor string) string {
	switch strings.ToLower(strings.TrimSpace(valor)) {
	case "alta":
		return "alta"
	case "media", "média":
		return "media"
	default:
		return "baixa"
	}
}
