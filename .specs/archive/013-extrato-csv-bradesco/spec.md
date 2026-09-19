# 013 — Leitor de extrato CSV do Bradesco

Status: concluída em 2026-09-19, validada com o extrato real (226 criados; reimportação: 226 duplicados)
Fase do plano: extensão da Fase 2 (spec 003 entregou o OFX; esta abre a
segunda porta de entrada de extrato).

## Motivo

O Internet Banking do Bradesco do autor **não oferece exportação OFX** — só
CSV. A spec 003 construiu um tokenizador OFX sobre fixtures sintéticas; ele
continua válido e testado, mas não há arquivo real para alimentá-lo. Sem esta
spec, o produto não recebe os dados reais que justificam sua existência.

Este é o risco nº 1 do plano se materializando de uma forma que não estava
prevista: a suposição não era "o parser está errado", era "o formato existe".

## O arquivo real

Exportado de conta corrente, UTF-8 **com BOM** (`EF BB BF`), `;` como
separador, 249 linhas:

```
Extrato de: Ag: NNNN | Conta: NNNNN-N;;;;;
Data;Histórico;Docto.;Crédito (R$);Débito (R$);Saldo (R$)
19/06/2026;COD. LANC. 0;0;0,00; ;0,00
19/06/2026;PIX RECEBIDO;1301495;342,85; ;342,85
17/09/2026;PIX ENVIADO;1741169; ;200,00;723,29

;;Total;1.375,75;1.015,99;723,29;
```

Fatos que o formato impõe:

* **Duas linhas de cabeçalho**, a primeira com agência e conta (PII).
* **Crédito e débito em colunas separadas**; a que não vale traz um espaço.
  O sinal não está no número — está na coluna. Débito vira valor negativo.
* **Sem FITID.** Não existe identificador de transação. É a diferença
  material para o OFX e o centro das decisões abaixo.
* **Sem hora.** Só `DD/MM/AAAA`.
* Rodapé `;;Total;...` e linha em branco no fim.
* `COD. LANC. 0` com `0,00` é linha informativa (saldo anterior).
* Históricos reais observados: `PIX ENVIADO`, `PIX RECEBIDO`,
  `PIX QR CODE ESTATICO`, `PIX QR CODE DINAMICO`, `DEVOLUCAO PIX`,
  `TRANSF SALDO C/SAL P/CC`, `TRANSF.MMA.TITULARIDADE*`,
  `SAQUE CARTAO TRANSF PIX*`, `PAGTO ELETRON  COBRANCA`,
  `GASTOS CARTAO DE CREDITO`, `EMPRESTIMO PESSOAL`,
  `PARCELA CREDITO PESSOAL`, `RENTAB.INVEST FACILCRED*`.

## Decisões e trade-offs

### 1. Origem própria (`extrato_csv` = 5), não reaproveitar `extrato_ofx`

Migração 009 acrescenta a linha na tabela `origens`. Custa uma migração, mas
a origem é o que distingue a qualidade da evidência: OFX tem identificador do
banco, CSV tem identificador **inventado por nós**. A conciliação de nível 2
(spec 007) pontua por origem, e misturar as duas apagaria essa diferença.
Também mantém honesta a auditoria: dá para responder "de onde veio esse
lançamento" sem adivinhar.

### 2. Identidade sintética, e o que ela não garante

Sem FITID, a impressão de idempotência nasce de
`data | histórico | documento | valor | ordinal`, sha256, hex.

O `ordinal` é a contagem de ocorrências **daquela tupla idêntica** dentro do
arquivo (0, 1, 2…). Sem ele, dois PIX de R$ 50,00 para o mesmo destino no
mesmo dia com o mesmo `Docto.` colapsariam num só — perder um lançamento real
é pior que arriscar um duplicado.

**O trade-off, dito por inteiro:** a estabilidade do ordinal depende de o
arquivo conter o dia inteiro. Se o autor exportar um recorte que começa no
meio de um dia, uma linha anterior some, o ordinal da seguinte muda, a
impressão muda e ela entra como novo lançamento. Na prática o `Docto.` é
quase sempre distinto e o ordinal fica em 0; a falha exige coincidência de
data, histórico, documento e valor **mais** um recorte parcial. Aceito e
documentado: a alternativa seria pedir sempre o mês fechado, o que é uma
regra que o autor esqueceria em seis meses.

Regra operacional, que vai para o README: **exportar sempre por mês fechado.**

### 3. Data às 00:00 no fuso de São Paulo

O CSV não tem hora. A competência (spec 001) pertence ao dia no fuso do
usuário, então meia-noite local é o instante correto e estável — não é um
valor inventado, é o único que o arquivo sustenta.

### 4. Heurística de meio, ampliada com vocabulário real

A tabela da 003 nasceu de formato documentado; agora há históricos de verdade.
Regras acrescentadas, todas por **palavra inteira**, nunca substring — o erro
de `"TED"` dentro de `LIMITED` da auditoria não se repete:

| Histórico contém | Meio |
|---|---|
| `PIX` (inclusive `DEVOLUCAO PIX`, `QR CODE`) | `pix` |
| `TRANSF`, `TED`, `DOC` | `transferencia` |
| `SAQUE` | `dinheiro` |
| `COBRANCA`, `BOLETO`, `TITULO` | `boleto` |
| `CARTAO` + `CREDITO`, `EMPRESTIMO`, `PARCELA` | `credito` |
| resto | `debito` |

`SAQUE CARTAO TRANSF PIX*` casa com três regras; a ordem do `switch` é a
precedência e está explícita no código — PIX ganha, porque é o trilho real do
dinheiro. Decisão discutível e por isso escrita aqui.

### 5. Reuso do `lerValor` da 003

`1.375,75` já é exatamente o caso que o parser de valor do OFX trata (milhar
com ponto, decimal com vírgula, validação de agrupamento, sem float). Nada de
segundo parser de dinheiro: um só caminho para centavos no repositório.

## Entregas

| # | Arquivo | Conteúdo |
|---|---|---|
| 1 | `migracoes/009_origem_extrato_csv.sql` | `INSERT INTO origens (5, 'extrato_csv')`. |
| 2 | `internal/dominio/ocorrencia/ocorrencia.go` | `OrigemExtratoCSV = 5` na lista fechada e em `EhValida`. |
| 3 | `internal/adaptadores/entrada/extrato/csv_bradesco.go` | `AnalisarCSV(dados, fuso)` → `[]Transacao`, reusando `Transacao`, `lerValor` e o teto de 8 MiB. `encoding/csv` com `Comma=';'`, `FieldsPerRecord=-1` (o rodapé tem 7 campos). |
| 4 | `internal/adaptadores/entrada/extrato/csv_bradesco_test.go` | Tabela sobre fixture **sintética** (`testdata/sintetico_bradesco.csv`): crédito, débito, linha zerada, rodapé, BOM, tupla repetida → ordinais distintos, valor mal formado → erro com posição e sem conteúdo. |
| 5 | `cmd/caixactl/main.go` | `importar` escolhe o parser pela extensão (`.csv` → CSV, resto → OFX) e passa a origem correspondente. |
| 6 | `README.md` | Tabela de pendências: OFX indisponível no Bradesco; instrução de exportar por mês fechado. |
| 7 | `docs/revisao-ia.md` | Entrada 006: parser escrito para um formato que o banco não exporta. |

## SEC-CHECK

* **IV (entrada não confiável)** — o CSV é entrada hostil: teto de 8 MiB
  reusado, `FieldsPerRecord=-1` com validação explícita de tamanho por linha,
  valor e data estritos, contraparte truncada em `ContraparteMaxima`.
* **Logging** — erro cita **posição da linha**, nunca o conteúdo: cada linha
  carrega valor e contraparte, que são PII. O `fmt.Printf` do `caixactl`
  continua imprimindo só contagens.
* **Secrets** — a primeira linha traz agência e conta. Ela é descartada no
  parse e **não** entra no payload; `extratos/` já está no `.gitignore`.
* **PQ** — nenhum SQL novo além do `INSERT` estático da migração.

## Verificação

```powershell
gofmt -l . ; go vet ./... ; go test ./...
. .\local.ps1 ; go run ./cmd/caixactl migrar
. .\local.ps1 ; go run ./cmd/caixactl importar extratos\<arquivo>.csv
# rodar duas vezes: a segunda deve reportar tudo como duplicado
```
