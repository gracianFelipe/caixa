// Tela "Mês" — dashboard no padrão Parthean (spec 014): hero lavanda com as
// saídas do mês, chip de pendentes, donut por categoria ("Para onde foi o
// dinheiro?"), heatmap diário ("Quando saiu?"), sinais e captura rápida.
// Lê /api/relatorio/{competencia} e /api/lancamentos; escreve só em
// POST /api/lancamentos.
import { api, ErroAPI, dinheiro, aoVivo, svg } from '../app.js';

const MEIOS = [
  ['pix', 'Pix'], ['credito', 'Crédito'], ['debito', 'Débito'],
  ['boleto', 'Boleto'], ['dinheiro', 'Dinheiro'], ['transferencia', 'Transferência'],
];

// Fatias do donut usam a paleta categorica dos tokens; 5 + "outros" no
// maximo (regra no-pie-overuse do guia de design).
const CORES_DONUT = ['--dado-1', '--dado-2', '--dado-3', '--dado-4', '--dado-5'];

function el(tag, atributos = {}, ...filhos) {
  const n = document.createElement(tag);
  for (const [k, v] of Object.entries(atributos)) {
    if (v === false || v == null) continue;
    if (k === 'class') n.className = v;
    else if (k === 'text') n.textContent = v;
    else if (k.startsWith('on') && typeof v === 'function') n.addEventListener(k.slice(2), v);
    else n.setAttribute(k, v === true ? '' : v);
  }
  n.append(...filhos.filter((f) => f != null));
  return n;
}

// svg() e markup do proprio app, nunca dado da API.
function icone(nome, classe = '') {
  const s = el('span', { class: classe, 'aria-hidden': 'true' });
  s.innerHTML = svg(nome);
  return s;
}

function garantirCss() {
  const href = new URL('./mes.css', import.meta.url).href;
  if (document.querySelector(`link[href="${href}"]`)) return;
  document.head.append(el('link', { rel: 'stylesheet', href }));
}

const TITULOS_DE_SINAL = {
  vazamento_por_repeticao: (s) => `${s.nome || 'sem categoria'}: ${s.severidade} gastos pequenos somam ${dinheiro.formatar(s.valor_centavos)}`,
  assinatura_esquecida: (s) => `${s.contraparte} cobra ${dinheiro.formatar(s.valor_centavos)} há ${s.severidade} meses`,
  categoria_em_escalada: (s) => `${s.nome || 'sem categoria'} subiu ${s.severidade}% em meses seguidos`,
  estouro_de_orcamento: (s) => `${s.nome || 'sem categoria'} em ${s.severidade}% do limite`,
  gasto_atipico: (s) => `${s.contraparte || s.nome}: ${dinheiro.formatar(s.valor_centavos)} fora do padrão`,
};

// Comparacao com o mes anterior em inteiros: (atual-anterior)*100/anterior.
function comparacao(atual, anterior) {
  if (anterior <= 0) return null;
  const pct = Math.trunc(((atual - anterior) * 100) / anterior);
  return { pct, subiu: pct > 0 };
}

function esqueleto() {
  return el('div', { class: 'mes', 'aria-busy': 'true' },
    el('div', { class: 'painel-hero mes-esqueleto' },
      el('div', { class: 'esqueleto mes-esqueleto--rotulo' }),
      el('div', { class: 'esqueleto mes-esqueleto--destaque' }),
      el('div', { class: 'esqueleto mes-esqueleto--linha' })),
    el('div', { class: 'cartao' },
      el('div', { class: 'esqueleto mes-esqueleto--linha' }),
      el('div', { class: 'esqueleto mes-esqueleto--barra' }),
      el('div', { class: 'esqueleto mes-esqueleto--linha-curta' })));
}

// --- donut em SVG proprio (sem biblioteca, contrato da spec 010) ----------

function donut(fatias) {
  // stroke-dasharray sobre circulo: cada fatia e um arco proporcional.
  const NS = 'http://www.w3.org/2000/svg';
  const R = 15.9155; // circunferencia ~100, entao pct vira comprimento direto
  const svgEl = document.createElementNS(NS, 'svg');
  svgEl.setAttribute('viewBox', '0 0 42 42');
  svgEl.setAttribute('class', 'mes-donut__grafico');
  svgEl.setAttribute('role', 'img');
  svgEl.setAttribute('aria-hidden', 'true'); // a legenda ao lado e o texto

  let deslocamento = 25; // comeca no topo
  for (const f of fatias) {
    const c = document.createElementNS(NS, 'circle');
    c.setAttribute('cx', '21');
    c.setAttribute('cy', '21');
    c.setAttribute('r', String(R));
    c.setAttribute('fill', 'none');
    c.setAttribute('stroke', `var(${f.cor})`);
    c.setAttribute('stroke-width', '5');
    c.setAttribute('stroke-dasharray', `${f.pct} ${100 - f.pct}`);
    c.setAttribute('stroke-dashoffset', String(deslocamento));
    svgEl.append(c);
    deslocamento -= f.pct;
  }
  return svgEl;
}

function cartaoDonut(rel, totalPendentes, ctx) {
  const cartao = el('section', { class: 'cartao mes-donut', 'aria-label': 'Gasto por categoria' },
    el('h2', { class: 'titulo-secao', text: 'Para onde foi o dinheiro?' }));

  if (rel.por_categoria.length === 0) {
    cartao.append(el('p', { class: 'rotulo', text: 'nenhum gasto categorizado ainda' }));
  } else {
    const total = rel.por_categoria.reduce((s, c) => s + c.total_centavos, 0) || 1;
    const principais = rel.por_categoria.slice(0, CORES_DONUT.length);
    const resto = rel.por_categoria.slice(CORES_DONUT.length);

    const fatias = principais.map((c, i) => ({
      nome: c.nome || 'sem categoria',
      valor: c.total_centavos,
      pct: Math.max(1, Math.round((c.total_centavos * 100) / total)),
      cor: CORES_DONUT[i],
    }));
    if (resto.length) {
      const soma = resto.reduce((s, c) => s + c.total_centavos, 0);
      fatias.push({ nome: 'outros', valor: soma, pct: Math.max(1, Math.round((soma * 100) / total)), cor: '--dado-resto' });
    }
    // Normaliza arredondamento para fechar 100.
    const excesso = fatias.reduce((s, f) => s + f.pct, 0) - 100;
    if (excesso !== 0) fatias[0].pct -= excesso;

    const legenda = el('ul', { class: 'mes-donut__legenda lista' },
      ...fatias.map((f) => el('li', { class: 'mes-donut__linha' },
        el('span', { class: 'mes-donut__cor', style: `background: var(${f.cor})`, 'aria-hidden': 'true' }),
        el('span', { class: 'mes-donut__pct valor', text: `${f.pct}%` }),
        el('span', { class: 'mes-donut__nome', text: f.nome }),
        el('span', { class: 'valor mes-donut__valor', text: dinheiro.formatar(f.valor) }))));

    const centro = el('div', { class: 'mes-donut__centro' },
      el('span', { class: 'rotulo', text: 'total' }),
      el('span', { class: 'valor', text: dinheiro.formatar(total) }));

    cartao.append(el('div', { class: 'mes-donut__corpo' },
      el('div', { class: 'mes-donut__anel' }, donut(fatias), centro),
      legenda));
  }

  if (totalPendentes > 0) {
    cartao.append(el('button', {
      class: 'mes-pendentes',
      type: 'button',
      onclick: () => ctx.navegar(`/lancamentos?competencia=${encodeURIComponent(ctx.competencia)}&filtro=sem-categoria`),
    },
      icone('editar', 'mes-pendentes__icone'),
      el('span', { text: `${totalPendentes} sem categoria — toque para classificar` }),
      icone('direita', 'mes-pendentes__seta')));
  }
  return cartao;
}

// --- heatmap de calendario (grid CSS, sem biblioteca) ----------------------

function cartaoHeatmap(lancamentos, comp) {
  const cartao = el('section', { class: 'cartao mes-calor', 'aria-label': 'Gasto por dia' },
    el('h2', { class: 'titulo-secao', text: 'Quando saiu?' }));

  const [ano, mesNum] = comp.split('-').map(Number);
  const dias = new Date(ano, mesNum, 0).getDate();
  const porDia = new Array(dias + 1).fill(0);
  for (const l of lancamentos) {
    if (l.valor_centavos >= 0) continue;
    const d = new Date(l.ocorrido_em);
    if (d.getFullYear() === ano && d.getMonth() + 1 === mesNum) {
      porDia[d.getDate()] += -l.valor_centavos;
    }
  }
  const maior = Math.max(...porDia);
  if (maior === 0) {
    cartao.append(el('p', { class: 'rotulo', text: 'nenhuma saída neste mês' }));
    return cartao;
  }

  // Intensidade em 4 degraus proporcionais ao maior dia do mes.
  const degrau = (v) => v === 0 ? 0 : Math.min(4, 1 + Math.trunc((v * 4) / (maior + 1)));

  const grade = el('div', { class: 'mes-calor__grade', role: 'list' });
  // Celulas vazias ate o dia da semana do dia 1 (semana comeca no domingo).
  const primeiro = new Date(ano, mesNum - 1, 1).getDay();
  for (let i = 0; i < primeiro; i++) grade.append(el('span', { class: 'mes-calor__vazio', 'aria-hidden': 'true' }));

  let diaMaior = 1;
  for (let d = 1; d <= dias; d++) {
    if (porDia[d] > porDia[diaMaior]) diaMaior = d;
    const celula = el('span', {
      class: `mes-calor__dia mes-calor__dia--${degrau(porDia[d])}`,
      role: 'listitem',
      'aria-label': `dia ${d}: ${porDia[d] ? dinheiro.formatar(porDia[d]) : 'sem saída'}`,
      title: `dia ${d}: ${porDia[d] ? dinheiro.formatar(porDia[d]) : '—'}`,
    }, el('span', { class: 'mes-calor__num', text: String(d) }));
    grade.append(celula);
  }
  cartao.append(grade);
  cartao.append(el('p', { class: 'rotulo mes-calor__resumo', text:
    `maior dia: ${diaMaior} (${dinheiro.formatar(porDia[diaMaior])})` }));
  return cartao;
}

export default {
  titulo: 'Mês',

  async montar(raiz, ctx) {
    garantirCss();

    let vivo = true;
    let temporizador = null;
    const cancelarAoVivo = aoVivo.assinar(() => {
      // Debounce: importacao despeja dezenas de eventos de uma vez.
      clearTimeout(temporizador);
      temporizador = setTimeout(() => { if (vivo) carregar(); }, 500);
    });

    async function carregar() {
      raiz.replaceChildren(esqueleto());
      let rel, lancs;
      try {
        [rel, lancs] = await Promise.all([
          api.get('/relatorio/' + ctx.competencia),
          api.get(`/lancamentos?competencia=${encodeURIComponent(ctx.competencia)}`),
        ]);
      } catch (e) {
        if (!vivo) return;
        raiz.replaceChildren(el('div', { class: 'vazio' },
          icone('alerta'),
          el('h2', { text: 'Não deu para carregar' }),
          el('p', { class: 'vazio__texto', text: e instanceof ErroAPI ? e.mensagem : 'sem conexão' }),
          el('button', { class: 'botao', type: 'button', onclick: carregar, text: 'Tentar de novo' })));
        return;
      }
      if (!vivo) return;
      renderizar(rel, lancs ?? []);
    }

    function renderizar(rel, lancs) {
      const teveMovimento = rel.total_saidas_centavos > 0 || rel.total_entradas_centavos > 0;
      const painel = el('div', { class: 'mes-painel' });

      // --- hero lavanda ---
      const destaque = el('section', { class: 'painel-hero mes-destaque', 'aria-label': 'Resumo do mês' },
        el('p', { class: 'rotulo mes-destaque__rotulo', text: 'saídas do mês' }),
        el('p', { class: 'valor valor--destaque', text: dinheiro.formatar(rel.total_saidas_centavos) }));

      const comp = comparacao(rel.total_saidas_centavos, rel.saidas_mes_anterior_centavos);
      if (comp) {
        destaque.append(el('p', { class: 'mes-comparacao ' + (comp.subiu ? 'mes-comparacao--sobe' : 'mes-comparacao--desce') },
          icone(comp.subiu ? 'direita' : 'esquerda', 'mes-comparacao__icone'),
          el('span', { text: `${comp.pct > 0 ? '+' : ''}${comp.pct}% vs ${dinheiro.formatar(rel.saidas_mes_anterior_centavos)} do mês anterior` })));
      } else {
        destaque.append(el('p', { class: 'mes-comparacao', text: 'sem base de comparação no mês anterior' }));
      }

      destaque.append(el('div', { class: 'mes-totais' },
        el('div', { class: 'mes-total' },
          el('span', { class: 'rotulo', text: 'entradas' }),
          el('span', { class: 'valor valor--entrada', text: dinheiro.formatar(rel.total_entradas_centavos) })),
        el('div', { class: 'mes-total' },
          el('span', { class: 'rotulo', text: 'saldo do mês' }),
          el('span', {
            class: 'valor ' + (rel.saldo_centavos >= 0 ? 'valor--entrada' : 'valor--saida'),
            text: dinheiro.formatar(rel.saldo_centavos),
          }))));
      painel.append(destaque);

      // --- donut + pendentes ---
      const pendentes = lancs.filter((l) => l.categoria_id == null).length;
      painel.append(cartaoDonut(rel, pendentes, ctx));

      // --- heatmap ---
      painel.append(cartaoHeatmap(lancs, ctx.competencia));

      // --- sinais ---
      const sinais = el('section', { class: 'cartao mes-sinais', 'aria-label': 'Sinais de alerta' },
        el('h2', { class: 'titulo-secao', text: 'O que merece atenção?' }));
      if (rel.sinais.length === 0) {
        sinais.append(el('p', { class: 'rotulo' },
          icone('ok'), el('span', { text: ' nenhum sinal de alerta' })));
      } else {
        for (const s of rel.sinais) {
          const grave = s.tipo === 'estouro_de_orcamento' || s.tipo === 'gasto_atipico';
          const texto = (TITULOS_DE_SINAL[s.tipo] || ((x) => x.detalhe))(s);
          sinais.append(el('p', { class: 'sinal ' + (grave ? 'sinal--perigo' : 'sinal--aviso') },
            icone(grave ? 'alerta' : 'aviso'),
            el('span', { text: texto })));
        }
      }
      painel.append(sinais);

      // --- captura rapida (progressive disclosure) ---
      const acoes = el('div', { class: 'mes-acoes' });
      const painelCaptura = montarCaptura(ctx, carregar);
      painelCaptura.classList.add('oculto');
      const alternar = el('button', {
        class: 'botao botao--primario mes-alternar',
        type: 'button',
        'aria-expanded': 'false',
        'aria-controls': 'captura-rapida',
        onclick: () => {
          const aberto = alternar.getAttribute('aria-expanded') === 'true';
          alternar.setAttribute('aria-expanded', String(!aberto));
          painelCaptura.classList.toggle('oculto', aberto);
          if (!aberto) painelCaptura.querySelector('input').focus();
        },
      }, icone('mais', 'mes-alternar__icone'), el('span', { text: 'Lançamento' }));
      acoes.append(alternar, painelCaptura);

      const tela = el('div', { class: 'mes' }, acoes, painel);
      if (!teveMovimento) {
        tela.append(el('div', { class: 'vazio mes-vazio' },
          icone('mes'),
          el('h2', { text: 'Mês sem lançamentos' }),
          el('p', { class: 'vazio__texto', text: 'registre um gasto acima ou importe um extrato com caixactl importar.' })));
      }
      raiz.replaceChildren(tela);
    }

    function montarCaptura(ctx, aoSalvar) {
      const erro = el('p', { class: 'mes-captura__erro', 'aria-live': 'polite' });
      const valor = el('input', { name: 'valor', inputmode: 'decimal', placeholder: '47,90', autocomplete: 'off' });
      const contraparte = el('input', { name: 'contraparte', placeholder: 'Padaria do Zé', autocomplete: 'off' });
      const meio = el('select', { name: 'meio' },
        ...MEIOS.map(([v, r]) => el('option', { value: v, text: r })));

      const salvar = el('button', { class: 'botao botao--primario', type: 'submit', text: 'Salvar' });

      const form = el('form', { class: 'cartao mes-captura', id: 'captura-rapida' },
        el('h2', { class: 'titulo-secao', text: 'novo lançamento' }),
        el('label', { class: 'campo' }, el('span', { text: 'valor (sem sinal = saída)' }), valor, erro),
        el('label', { class: 'campo' }, el('span', { text: 'contraparte' }), contraparte),
        el('label', { class: 'campo' }, el('span', { text: 'meio' }), meio),
        el('div', { class: 'mes-captura__acoes' }, salvar));

      form.addEventListener('submit', async (ev) => {
        ev.preventDefault();
        erro.textContent = '';
        let centavos;
        try {
          centavos = dinheiro.analisar(valor.value);
          if (centavos === 0) throw new Error('valor não pode ser zero');
          // Sem sinal explicito = saida: captura rapida existe para gasto.
          if (centavos > 0 && !valor.value.trim().startsWith('+')) centavos = -centavos;
        } catch (e) {
          erro.textContent = e.message;
          valor.focus();
          return;
        }
        if (!contraparte.value.trim()) {
          erro.textContent = 'informe a contraparte';
          contraparte.focus();
          return;
        }

        salvar.disabled = true;
        try {
          await api.post('/lancamentos', {
            valor_centavos: centavos,
            meio: meio.value,
            contraparte: contraparte.value.trim(),
            ocorrido_em: new Date().toISOString(),
          });
          ctx.toast('lançamento registrado', 'sucesso');
          form.reset();
          aoSalvar();
        } catch (e) {
          erro.textContent = e instanceof ErroAPI ? e.mensagem : 'não deu para salvar';
        } finally {
          salvar.disabled = false;
        }
      });
      return form;
    }

    await carregar();

    return () => {
      vivo = false;
      clearTimeout(temporizador);
      cancelarAoVivo();
    };
  },
};
