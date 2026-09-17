# Checklist de Segurança (SEC-CHECK)

> Ideia: assim como você já usa SOLID/KISS/DRY/DDD/TDD como "vocabulário" pra guiar o código,
> este arquivo é o vocabulário equivalente pra segurança. Coloque este arquivo (ou um resumo dele)
> no `CLAUDE.md` do projeto, no system prompt do seu chat, ou num `SECURITY.md` na raiz do repo.
> Toda vez que pedir pro Claude (ou outra IA) mexer em código, referencie as siglas — ex:
> "aplique SEC-CHECK: Headers, PQ, IV/OE, Secrets".

---

## 0. PROMPT UNIVERSAL — cole junto de QUALQUER script, em qualquer linguagem

> Esta versão junta segurança (camada A) com princípios de design (camada B).
> A IA precisa **avaliar o contexto do script antes de aplicar qualquer princípio de B**
> — nem todo script pede SOLID, DDD ou TDD, do mesmo jeito que nem todo script serve
> HTTP e precisa de CSP/HSTS.

```
Revise este código em duas camadas: (A) segurança e (B) princípios de design.
Antes de aplicar qualquer item, avalie o contexto real do script (tamanho, se é
lógica de domínio ou script utilitário/glue code, se é orientado a objetos, se já
tem suíte de teste, se roda uma vez ou é mantido a longo prazo) e aplique SOMENTE
o que fizer sentido nesse contexto. Não force um princípio que não se encaixa, e
não invente problema onde não há.

(A) SEC-CHECK — segurança
1. Secrets: alguma credencial, chave, token ou senha hardcoded? Deve virar
   variável de ambiente/secret manager. Nunca logar segredo.
2. Injection (PQ): query de banco, comando de shell/SO, ou eval de string dinâmica
   feita com input externo? Deve usar parametrização/prepared statement, nunca
   concatenação direta.
3. Input Validation (IV): todo dado que entra (param, body, header, arquivo, env)
   tem validação de tipo, formato, tamanho e whitelist?
4. Output Encoding (OE): todo dado que sai pra HTML, SQL, shell, log ou outro
   contexto de interpretação está escapado/sanitizado?
5. AuthN/AuthZ (PoLP): toda ação sensível checa autenticação E autorização no
   servidor? Nunca confiar em flag/role vinda do client.
6. Fail Secure: erro vaza stack trace, path do sistema ou versão de lib pro
   usuário final? Log detalhado deve ficar só no servidor.
7. Deserialização/Parsing: desserializa objeto de fonte não confiável sem
   validação? (risco de RCE)
8. Dependências: alguma lib visivelmente desatualizada ou com CVE conhecida que
   você reconheça?
9. Transporte/Headers (se for app web): CSP, HSTS, X-Frame-Options,
   X-Content-Type-Options presentes? Cookies com Secure/HttpOnly/SameSite?
10. Rate limiting: endpoint sensível (login, reset de senha, upload) protegido
    contra abuso/força bruta?
11. IDOR/SSRF: acesso a recurso por ID confere posse do usuário logado?
    Requisição a URL externa usa allowlist de destino?
12. Logging: log contém senha, token, PII ou número de cartão?

(B) Princípios de design — avalie a pertinência de CADA UM antes de aplicar:
- SOLID (SRP/OCP/LSP/ISP/DIP): só relevante se houver classes/módulos com
  responsabilidade crescendo, herança, ou implementações trocáveis. Script
  procedural pequeno geralmente não precisa.
- KISS: quase sempre relevante — não deixe a solução mais complexa que o
  problema pede, mas também não force isso a virar desculpa pra abstrair demais.
- DRY: elimine duplicação real (mesma regra de negócio repetida). Não elimine
  coincidência superficial (dois trechos parecidos sem acoplamento semântico).
- YAGNI: relevante sempre que houver generalização/flexibilidade especulativa
  que ninguém pediu ainda.
- DDD: só relevante em lógica de negócio complexa com entidades e regras ricas.
  Não force em script utilitário, CRUD simples ou glue code.
- TDD: só relevante se o projeto já tem suíte de teste ou se o pedido inclui
  escrever/alterar testes. Não force teste em script descartável/one-off.
- LoD (Law of Demeter): só relevante em código orientado a objetos com grafo de
  dependências (evitar encadeamento tipo `a.b.c.d()`).
- TDA (Tell, Don't Ask): só relevante em OO onde objetos deveriam decidir sobre
  seu próprio estado em vez de expor esse estado pra decisão externa.
- CoC (Convention over Configuration): só relevante se o projeto usa um
  framework com convenções estabelecidas (Rails, NestJS, Django etc.).
- SoC (Separation of Concerns): relevante em qualquer coisa acima de um script
  trivial de poucas linhas.

Responda em 4 blocos:
🧭 Princípios aplicáveis (liste os que você vai usar de (B) e, em 1 linha, por que
   fazem sentido pra ESTE script; liste também os que está deliberadamente
   ignorando e por quê)
⚠️ Problemas encontrados — segurança e design (com linha/trecho)
✅ O que já está correto
🔧 Correção aplicada (se reescrever o código, mostre o diff)
```

---

## 1. O que o workflow do seu amigo faz (resumo)

Pipeline de CI (GitHub Actions) com 4 camadas, cada uma cobrindo um tipo de risco diferente:

| Ferramenta | Categoria | O que pega |
|---|---|---|
| **Gitleaks** | Secret Scanning | Senhas, tokens, chaves de API commitados no código |
| **Semgrep** | SAST (Static Application Security Testing) | Bugs de segurança no próprio código-fonte (SQLi, XSS, lógica insegura) |
| **Trivy** | SCA (Software Composition Analysis) | Vulnerabilidades conhecidas (CVE) em dependências (`node_modules`) e misconfig |
| **OWASP ZAP** | DAST (Dynamic Application Security Testing) | Ataca a aplicação **rodando** de verdade — headers, cookies, respostas HTTP |

Pontos importantes que ele já fez certo (mantenha sempre):
- **Hashes imutáveis** nas imagens Docker (`@sha256:...`) em vez de `:latest` — evita que a imagem mude por baixo do seu pé.
- **`persist-credentials: false`** no checkout — o job não guarda token do GitHub além do necessário.
- **`permissions: contents: read`** no topo — permissão mínima (PoLP) pro workflow inteiro.
- **`exit-code 1`** nos scanners — achado relevante **quebra o CI**, não é só um aviso ignorável.
- ZAP só roda contra `http://127.0.0.1:3000` (a própria app, localmente) — nunca contra sistema de terceiros sem autorização.

---

## 2. Prompt pronto pra colar no seu chat/CLAUDE.md

```
SEC-CHECK obrigatório neste projeto. Ao gerar ou revisar código, aplique:
- Headers: CSP, HSTS, XFO, XCTO, Referrer-Policy, Permissions-Policy
- PQ: toda query SQL parametrizada, nunca concatenar string com input do usuário
- IV/OE: validar todo input de entrada e sanitizar/escapar toda saída (XSS)
- Secrets: zero credencial hardcoded; usar env vars / secrets manager; nunca logar segredo
- AuthN/AuthZ: PoLP em toda role/permissão; nunca confiar em input do client pra autorização
- CORS: allowlist explícita de origem, nunca `*` com credentials
- Errors: fail secure (erro genérico pro usuário, log detalhado só no server)
- Deps: nenhuma lib com CVE HIGH/CRITICAL conhecida sem justificativa
- Logs: nunca logar senha, token, PII, número de cartão
Se algum código que você gerar violar um desses pontos, avise explicitamente antes de entregar.
```

---

## 3. Dicionário de siglas (pra você e pra IA entenderem igual)

### 3.1 Princípios gerais (nível arquitetura — paralelo ao SOLID)

| Sigla | Nome completo | O que significa na prática |
|---|---|---|
| **PoLP** | Principle of Least Privilege | Cada usuário/serviço/token só tem acesso ao mínimo necessário |
| **DiD** | Defense in Depth | Várias camadas de proteção — se uma falha, outra segura |
| **SbD** | Secure by Default | Configuração padrão já vem segura (ex: cookie `Secure`/`HttpOnly` por padrão) |
| **FS** | Fail Secure / Fail Closed | Se algo quebrar, o sistema nega acesso — não libera por engano |
| **ZT** | Zero Trust | Nunca confiar implicitamente em rede interna, sempre validar |
| **CIA** | Confidentiality, Integrity, Availability | As 3 propriedades que toda decisão de segurança protege |
| **AAA** | Authentication, Authorization, Auditing | Quem é você, o que você pode, o que você fez |
| **SLS** | Shift-Left Security | Segurança entra no código cedo (no dev/PR), não só em produção |

### 3.2 No nível do código (o que você escreve todo dia)

| Sigla | Nome completo | Regra prática |
|---|---|---|
| **IV** | Input Validation | Validar tipo, formato, tamanho, whitelist de todo input |
| **OE** | Output Encoding | Escapar dado antes de renderizar (evita XSS) |
| **PQ** | Parameterized Queries | Nunca montar SQL/NoSQL com concatenação de string (evita Injection) |
| **CSRF-P** | CSRF Protection | Token anti-CSRF em toda ação que muda estado, via cookie de sessão |
| **IDOR** | Insecure Direct Object Reference | Sempre checar se o usuário logado **pode** acessar aquele ID específico |
| **SSRF-P** | SSRF Protection | Nunca deixar o server fazer request pra URL vinda direto do usuário sem allowlist |
| **Rate-L** | Rate Limiting | Limitar tentativas em login, reset de senha, endpoints públicos |

### 3.3 Headers HTTP (o que o ZAP checa) — IDs confirmados na doc do ZAP

| ID ZAP | Header/Regra | Pra que serve |
|---|---|---|
| **10020** | X-Frame-Options / CSP `frame-ancestors` | Anti-clickjacking (bloqueia seu site num `<iframe>` alheio) |
| **10021** | X-Content-Type-Options: nosniff | Impede o navegador de "adivinhar" tipo de arquivo (MIME sniffing) |
| **10038** | Content-Security-Policy | Restringe de onde o navegador pode carregar script/estilo/imagem |
| **10035** | Strict-Transport-Security (HSTS) | Força HTTPS sempre, nunca cai pra HTTP |
| **10063** | Permissions-Policy | Desliga APIs do navegador que a app não usa (câmera, geo, etc.) |
| **10054** | Cookie sem `SameSite` | Cookie vulnerável a CSRF cross-site |
| **10098** | CORS mal configurado | Origem liberada demais (ex: `*` com credenciais) |
| **10036/10037** | Server / X-Powered-By | Vaza versão de framework/servidor pra atacante |
| **10062** | PII Disclosure | Dado pessoal aparecendo onde não devia (resposta, log, erro) |

> Esses IDs são das *passive scan rules* do ZAP — como o ZAP é atualizado com frequência, confirme sempre na doc oficial (`zaproxy.org/docs/alerts`) antes de assumir que um número específico ainda existe naquela versão.

### 3.4 Categorias de ferramentas (pra saber o que cada scanner do seu pipeline resolve)

| Sigla | Nome | Exemplo no seu workflow |
|---|---|---|
| **SAST** | Static Application Security Testing | Semgrep — analisa código sem rodar |
| **DAST** | Dynamic Application Security Testing | ZAP — ataca a app rodando |
| **SCA** | Software Composition Analysis | Trivy — vulnerabilidade em dependência de terceiro |
| **Secret Scan** | — | Gitleaks — credencial vazada no histórico do git |
| **SBOM** | Software Bill of Materials | Lista de tudo que sua app depende (bom pra auditoria) |

---

## 4. Referência rápida — OWASP Top 10 (2021)

| Código | Categoria |
|---|---|
| A01 | Broken Access Control |
| A02 | Cryptographic Failures |
| A03 | Injection |
| A04 | Insecure Design |
| A05 | Security Misconfiguration |
| A06 | Vulnerable and Outdated Components |
| A07 | Identification and Authentication Failures |
| A08 | Software and Data Integrity Failures |
| A09 | Security Logging and Monitoring Failures |
| A10 | Server-Side Request Forgery (SSRF) |

---

## 5. Como usar isso no dia a dia

1. Cole a **seção 2** (o prompt) no `CLAUDE.md` do repo ou nas instruções do projeto do Claude Code/Claude.ai.
2. Quando pedir uma feature, adicione no fim: *"aplique SEC-CHECK, foco em PQ e IV/OE"* (ou a sigla relevante pro contexto — ex: endpoint novo → CORS + Rate-L + AuthZ).
3. Mantenha o workflow de CI travando o build (`exit-code 1`) — é o que garante que a regra não depende de ninguém lembrar manualmente.
4. Uma vez por trimestre, confira se as versões/hashes das imagens Docker do workflow (Gitleaks, Semgrep, Trivy, ZAP) ainda são as mais recentes.
