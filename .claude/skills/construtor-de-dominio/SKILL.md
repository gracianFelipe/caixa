---
name: construtor-de-dominio
description: Como este repo escreve um tipo de domínio novo em internal/dominio — tipo próprio, construtor Novo com invariantes, erros sentinela, String/Analisar com ida e volta, teste table-driven. Use ao criar ou estender qualquer pacote de domínio.
---

# Construtor de domínio (padrão do Caixa)

Destilado de `internal/dominio/dinheiro`, `competencia`, `identidade` e
`lancamento`, escritos à mão na spec 001. Leia um deles antes de escrever.

## Forma

1. **Tipo próprio sobre tipo base** quando o valor é um só:
   `type Centavos int64`, `type Meio string`, `type ID [16]byte`. Struct só
   quando há mais de um campo; campos privados quando não podem ser alterados
   por fora (`Competencia{ano, mes}` + acessores).
2. **Erros sentinela no topo**, um `var (...)` com `errors.New("pacote: msg")`.
   Mensagem minúscula, sem PII, prefixada pelo pacote.
3. **Construtor `Novo`/`Nova`** com assinatura `(T, error)`. Valida tudo antes
   de montar; devolve `T{}` zero junto com o erro. Struct de entrada (`Dados`)
   quando passam de três parâmetros.
4. **Tempo, fuso e aleatoriedade entram como parâmetro** — nunca `time.Now()`,
   `time.LoadLocation`, `rand.Reader` implícitos.
5. **Só stdlib.** Se faltar algo (acentos, UUID), escreve-se o mínimo à mão.
6. **`String()` e `Analisar(string)`** para todo tipo com forma textual. O par
   é inverso um do outro.
7. Receptor de **valor** por padrão; ponteiro só se muta ou se o struct é grande.

## Teste (mesmo arquivo `_test.go`, mesmo pacote)

* Tabela `[]struct{ nome string; ...; querido T; erro error }` com
  `t.Run(c.nome, ...)`.
* Erros comparados com `errors.Is`, nunca `==` nem pela mensagem.
* `TestIdaEVolta`: `Analisar(v.String()) == v` para uma lista de valores.
* Parser de entrada externa ganha `Fuzz*`: sem panic, nunca valor junto com
  erro, ida e volta para tudo que for aceito.
* Fuso nos testes: `time.FixedZone("America/Sao_Paulo", -3*60*60)`.
* Structs comparados com `cmp.Diff`; `cmp.AllowUnexported` quando há campo
  privado.

## Checagem antes de fechar

```bash
go test ./internal/dominio/...
go list -deps ./internal/dominio/... | grep -v '^github.com/gracianFelipe/caixa/' | grep -vE '^[a-z0-9_/]+$' && exit 1
```
