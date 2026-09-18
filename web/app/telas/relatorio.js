// Tela "Relatório": resumo do mês, sinais dos detectores e o texto bruto
// (o mesmo que vai ao Telegram). Só leitura: GET /api/relatorio/{competencia}.
import { api, ErroAPI, dinheiro, competencia as comp, aoVivo, svg } from '../app.js';

const TITULOS = {
  vazamento_por_repeticao: 'Gastos pequenos que somam alto',
  assinatura_esquecida: 'Assinatura esquecida?',
  categoria_em_escalada: 'Categoria em escalada',
  estouro_de_orcamento: 'Orçamento estourado',
  gasto_atipico: 'Gasto fora do padrão',
};

// Estouro e gasto atípico pedem ação imediata; os demais são avisos.
const PERIGO = new Set(['estouro_de_orcamento', 'gasto_atipico']);

// O campo "severidade" é um inteiro cujo significado depende do tipo.
function severidadeLegivel(sinal) {
  const n = Number(sinal.severidade) || 0;
  switch (sinal.tipo) {
    case 'vazamento_por_repeticao':
      return n === 1 ? '1 gasto' : `${n} gastos`;
    case 'assinatura_esquecida':
      return n === 1 ? '1 mês' : `${n} meses`;
    case 'categoria_em_escalada':
      return `${n >= 0 ? '+' : '−'}${Math.abs(n)}%`;
    case 'estouro_de_orcamento':
      return `${n}% do limite`;
    case 'gasto_atipico':
      return `${n}× o habitual`;
    default:
      return String(n);
  }
}

// Variação percentual inteira entre dois valores em centavos; null sem base.
function variacaoPercentual(atual, anterior) {
  if (!anterior) return null;
  return Math.round(((atual - anterior) * 100) / anterior);
}

function el(tag, atributos = {}, ...filhos) {
  const n = document.createElement(tag);
  for (const [k, v] of Object.entries(atributos)) {
    if (v == null || v === false) continue;
    if (k === 'class') n.className = v;
    else if (k === 'text') n.textContent = v;
    else n.setAttribute(k, v === true ? '' : v);
  }
  for (const f of filhos) {
    if (f == null) continue;
    n.append(f instanceof Node ? f : document.createTextNode(String(f)));
  }
  return n;
}

// svg() devolve markup gerado pelo próprio app (não é dado da API).
function icone(nome, classe = '') {
  const s = el('span', { class: `icone ${classe}`.trim(), 'aria-hidden': 'true' });
  s.innerHTML = svg(nome);
  return s;
}

function garantirEstilo() {
  const id = 'estilo-tela-relatorio';
  if (document.getElementById(id)) return;
  const link = el('link', { id, rel: 'stylesheet' });
  link.href = new URL('./relatorio.css', import.meta.url).href;
  document.head.append(link);
}

function metrica(rotulo, centavos, { destaque = false, entrada = false, id } = {}) {
  const classes = ['valor'];
  if (destaque) classes.push('valor--destaque');
  classes.push(entrada ? 'valor--entrada' : 'valor--saida');
  return el(
    'div',
    { class: `relatorio__metrica${destaque ? ' relatorio__metrica--principal' : ''}` },
    el('span', { class: 'rotulo', id, text: rotulo }),
    el('span', { class: classes.join(' '), 'aria-labelledby': id, text: dinheiro.formatar(centavos) }),
  );
}

function secaoResumo(rel, competenciaAtual) {
  const saidas = Number(rel.total_saidas_centavos) || 0;
  const entradas = Number(rel.total_entradas_centavos) || 0;
  const saldo = Number(rel.saldo_centavos) || 0;
  const anterior = Number(rel.saidas_mes_anterior_centavos) || 0;

  const grade = el(
    'div',
    { class: 'cartao relatorio__grade' },
    metrica('Saídas', saidas, { destaque: true, id: 'rel-saidas' }),
    metrica('Entradas', entradas, { entrada: true, id: 'rel-entradas' }),
    metrica('Saldo', saldo, { entrada: saldo >= 0, id: 'rel-saldo' }),
    metrica('Saídas no mês anterior', anterior, { id: 'rel-anterior' }),
  );

  // Comparação em barras CSS puras (sem biblioteca de gráfico).
  const rotuloAnterior = comp.rotulo(comp.anterior(competenciaAtual));
  const maior = Math.max(saidas, anterior, 1);
  const variacao = variacaoPercentual(saidas, anterior);
  let frase;
  if (variacao === null) frase = `Sem saídas registradas em ${rotuloAnterior} para comparar.`;
  else if (variacao === 0) frase = `Mesmo nível de saídas de ${rotuloAnterior}.`;
  else frase = `${variacao > 0 ? '+' : '−'}${Math.abs(variacao)}% em relação a ${rotuloAnterior}.`;

  const linhaBarra = (rotulo, valor) => {
    const preenchido = el('span', { class: 'barra__preenchido' });
    preenchido.style.setProperty('--pct', `${Math.round((valor * 100) / maior)}%`);
    return el(
      'div',
      { class: 'relatorio__linha-barra' },
      el('span', { class: 'rotulo', text: rotulo }),
      el('span', { class: 'barra', role: 'presentation' }, preenchido),
      el('span', { class: 'valor relatorio__valor-barra', text: dinheiro.formatar(valor) }),
    );
  };

  const comparacao = el(
    'div',
    {
      class: 'cartao relatorio__comparacao',
      role: 'group',
      'aria-label': `Saídas: ${comp.rotulo(competenciaAtual)} comparado a ${rotuloAnterior}`,
    },
    linhaBarra('Este mês', saidas),
    linhaBarra('Mês anterior', anterior),
    el('p', { class: `relatorio__variacao${variacao > 0 ? ' relatorio__variacao--alta' : ''}`, text: frase }),
  );

  return el(
    'section',
    { class: 'relatorio__secao', 'aria-labelledby': 'rel-titulo-resumo' },
    el('h2', { class: 'titulo-secao', id: 'rel-titulo-resumo', text: 'Resumo' }),
    grade,
    comparacao,
  );
}

function cartaoSinal(sinal) {
  const perigo = PERIGO.has(sinal.tipo);
  const titulo = TITULOS[sinal.tipo] || 'Sinal';
  const principal = sinal.contraparte || sinal.nome || '';
  const secundario = sinal.contraparte && sinal.nome ? sinal.nome : null;

  return el(
    'li',
    { class: `cartao relatorio__sinal ${perigo ? 'relatorio__sinal--perigo' : 'relatorio__sinal--aviso'}` },
    el(
      'div',
      { class: 'relatorio__sinal-cabecalho' },
      icone(perigo ? 'alerta' : 'aviso', 'relatorio__sinal-icone'),
      el('h3', { class: 'relatorio__sinal-titulo', text: titulo }),
      el('span', { class: `sinal ${perigo ? 'sinal--perigo' : 'sinal--aviso'}`, text: severidadeLegivel(sinal) }),
    ),
    el(
      'div',
      { class: 'relatorio__sinal-corpo' },
      el(
        'div',
        { class: 'relatorio__sinal-quem' },
        el('span', { class: 'relatorio__sinal-nome', text: principal }),
        secundario ? el('span', { class: 'rotulo', text: secundario }) : null,
      ),
      el('span', { class: 'valor valor--saida', text: dinheiro.formatar(Number(sinal.valor_centavos) || 0) }),
    ),
    sinal.detalhe ? el('p', { class: 'relatorio__sinal-detalhe', text: sinal.detalhe }) : null,
  );
}

function secaoSinais(sinais) {
  const lista = Array.isArray(sinais) ? sinais : [];
  let conteudo;
  if (lista.length === 0) {
    conteudo = el(
      'div',
      { class: 'vazio relatorio__vazio' },
      icone('ok', 'relatorio__vazio-icone'),
      el('h3', { class: 'vazio__titulo', text: 'Nenhum sinal neste mês' }),
      el('p', { class: 'vazio__texto', text: 'Os detectores não encontraram nada fora do padrão.' }),
    );
  } else {
    // Mais grave primeiro; perigo antes de aviso quando a severidade empata.
    const ordenados = [...lista].sort((a, b) => {
      const pa = PERIGO.has(a.tipo) ? 1 : 0;
      const pb = PERIGO.has(b.tipo) ? 1 : 0;
      return pb - pa || (Number(b.severidade) || 0) - (Number(a.severidade) || 0);
    });
    conteudo = el('ul', { class: 'relatorio__sinais' }, ...ordenados.map(cartaoSinal));
  }
  const quantidade = lista.length === 1 ? '1 sinal' : `${lista.length} sinais`;
  return el(
    'section',
    { class: 'relatorio__secao', 'aria-labelledby': 'rel-titulo-sinais' },
    el(
      'div',
      { class: 'relatorio__cabecalho-secao' },
      el('h2', { class: 'titulo-secao', id: 'rel-titulo-sinais', text: 'Sinais' }),
      el('span', { class: 'rotulo', text: quantidade }),
    ),
    conteudo,
  );
}

function secaoTexto(texto, toast) {
  const conteudo = typeof texto === 'string' ? texto : '';
  const botao = el('button', { type: 'button', class: 'botao botao--fantasma relatorio__copiar' }, 'Copiar texto');
  botao.addEventListener('click', async () => {
    if (!navigator.clipboard || !navigator.clipboard.writeText) {
      toast('Copiar não está disponível neste navegador.', 'erro');
      return;
    }
    try {
      await navigator.clipboard.writeText(conteudo);
      toast('Texto do relatório copiado.', 'sucesso');
    } catch {
      toast('Não foi possível copiar o texto.', 'erro');
    }
  });
  return el(
    'section',
    { class: 'relatorio__secao', 'aria-labelledby': 'rel-titulo-texto' },
    el('h2', { class: 'titulo-secao oculto-visualmente', id: 'rel-titulo-texto', text: 'Texto do relatório' }),
    el(
      'details',
      { class: 'cartao relatorio__texto' },
      el('summary', { class: 'relatorio__texto-resumo' }, 'Texto como no Telegram'),
      el('div', { class: 'relatorio__texto-acoes' }, botao),
      el('pre', { class: 'relatorio__pre', text: conteudo || 'Relatório sem texto para este mês.' }),
    ),
  );
}

function esqueleto() {
  const bloco = (classe) => el('div', { class: `esqueleto ${classe}` });
  return el(
    'div',
    { class: 'relatorio__esqueleto', 'aria-hidden': 'true' },
    bloco('relatorio__esqueleto-titulo'),
    bloco('relatorio__esqueleto-cartao'),
    bloco('relatorio__esqueleto-titulo'),
    bloco('relatorio__esqueleto-cartao'),
    bloco('relatorio__esqueleto-cartao'),
  );
}

function estadoErro(mensagem, tentarDeNovo) {
  const botao = el('button', { type: 'button', class: 'botao botao--primario' }, 'Tentar de novo');
  botao.addEventListener('click', tentarDeNovo);
  return el(
    'div',
    { class: 'vazio relatorio__vazio' },
    icone('alerta', 'relatorio__vazio-icone relatorio__vazio-icone--perigo'),
    el('h3', { class: 'vazio__titulo', text: 'Não deu para carregar o relatório' }),
    el('p', { class: 'vazio__texto', text: mensagem }),
    botao,
  );
}

export default {
  titulo: 'Relatório',

  async montar(raiz, ctx) {
    garantirEstilo();
    let ativo = true;
    // Descarta respostas atrasadas quando uma nova carga começou.
    let versao = 0;

    const status = el('div', { class: 'oculto-visualmente', role: 'status', 'aria-live': 'polite' });
    const conteudo = el('div', { class: 'relatorio' });
    raiz.replaceChildren(status, conteudo);

    async function carregar() {
      const minha = ++versao;
      conteudo.setAttribute('aria-busy', 'true');
      conteudo.replaceChildren(esqueleto());
      status.textContent = 'Carregando relatório…';
      try {
        const rel = await api.get(`/relatorio/${encodeURIComponent(ctx.competencia)}`);
        if (!ativo || minha !== versao) return;
        conteudo.replaceChildren(
          secaoResumo(rel || {}, ctx.competencia),
          secaoSinais(rel ? rel.sinais : []),
          secaoTexto(rel ? rel.texto : '', ctx.toast),
        );
        status.textContent = `Relatório de ${comp.rotulo(ctx.competencia)} carregado.`;
      } catch (e) {
        if (!ativo || minha !== versao) return;
        if (e instanceof ErroAPI && e.status === 401) return; // api já redirecionou ao login
        const mensagem = e instanceof ErroAPI && e.mensagem ? e.mensagem : 'Verifique a conexão e tente novamente.';
        conteudo.replaceChildren(estadoErro(mensagem, carregar));
        status.textContent = 'Falha ao carregar o relatório.';
      } finally {
        if (ativo && minha === versao) conteudo.removeAttribute('aria-busy');
      }
    }

    const cancelarAoVivo = aoVivo.assinar(({ tipo }) => {
      if (tipo === 'lancamento_criado') carregar();
    });

    await carregar();

    return () => {
      ativo = false;
      cancelarAoVivo();
    };
  },
};
