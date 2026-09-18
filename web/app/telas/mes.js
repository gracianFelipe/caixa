// Tela "Mês" — a principal: destaque de saídas com comparação, entradas e
// saldo, barras por categoria, sinais e captura rápida de lançamento.
// Lê /api/relatorio/{competencia}; escreve só em POST /api/lancamentos.
import { api, ErroAPI, dinheiro, aoVivo, svg } from '../app.js';

const MEIOS = [
  ['pix', 'Pix'], ['credito', 'Crédito'], ['debito', 'Débito'],
  ['boleto', 'Boleto'], ['dinheiro', 'Dinheiro'], ['transferencia', 'Transferência'],
];

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
    el('div', { class: 'cartao mes-esqueleto' },
      el('div', { class: 'esqueleto mes-esqueleto--rotulo' }),
      el('div', { class: 'esqueleto mes-esqueleto--destaque' }),
      el('div', { class: 'esqueleto mes-esqueleto--linha' })),
    el('div', { class: 'cartao' },
      el('div', { class: 'esqueleto mes-esqueleto--linha' }),
      el('div', { class: 'esqueleto mes-esqueleto--barra' }),
      el('div', { class: 'esqueleto mes-esqueleto--linha-curta' })));
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
      let rel;
      try {
        rel = await api.get('/relatorio/' + ctx.competencia);
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
      renderizar(rel);
    }

    function renderizar(rel) {
      const teveMovimento = rel.total_saidas_centavos > 0 || rel.total_entradas_centavos > 0;

      const painel = el('div', { class: 'mes-painel' });

      // --- destaque ---
      const destaque = el('section', { class: 'cartao mes-destaque', 'aria-label': 'Resumo do mês' },
        el('p', { class: 'rotulo', text: 'saídas do mês' }),
        el('p', { class: 'valor valor--destaque', text: dinheiro.formatar(rel.total_saidas_centavos) }));

      const comp = comparacao(rel.total_saidas_centavos, rel.saidas_mes_anterior_centavos);
      if (comp) {
        const p = el('p', { class: 'mes-comparacao ' + (comp.subiu ? 'mes-comparacao--sobe' : 'mes-comparacao--desce') },
          icone(comp.subiu ? 'direita' : 'esquerda', 'mes-comparacao__icone'),
          el('span', { text: `${comp.pct > 0 ? '+' : ''}${comp.pct}% vs ${dinheiro.formatar(rel.saidas_mes_anterior_centavos)} do mês anterior` }));
        destaque.append(p);
      } else {
        destaque.append(el('p', { class: 'mes-comparacao', text: 'sem base de comparação no mês anterior' }));
      }

      destaque.append(el('div', { class: 'mes-totais' },
        el('p', {},
          el('span', { class: 'rotulo', text: 'entradas ' }),
          el('span', { class: 'valor valor--entrada', text: dinheiro.formatar(rel.total_entradas_centavos) })),
        el('p', {},
          el('span', { class: 'rotulo', text: 'saldo ' }),
          el('span', {
            class: 'valor ' + (rel.saldo_centavos >= 0 ? 'valor--entrada' : 'valor--saida'),
            text: dinheiro.formatar(rel.saldo_centavos),
          }))));
      painel.append(destaque);

      // --- por categoria ---
      const categorias = el('section', { class: 'cartao mes-categorias', 'aria-label': 'Gasto por categoria' },
        el('h2', { class: 'titulo-secao', text: 'por categoria' }));
      if (rel.por_categoria.length === 0) {
        categorias.append(el('p', { class: 'rotulo', text: 'nenhum gasto categorizado ainda' }));
      } else {
        const maior = rel.por_categoria[0].total_centavos || 1;
        for (const c of rel.por_categoria) {
          const pct = Math.max(2, Math.trunc((c.total_centavos * 100) / maior));
          const preenchido = el('div', { class: 'barra__preenchido' });
          preenchido.style.setProperty('--pct', pct + '%');
          categorias.append(el('div', { class: 'mes-categoria' },
            el('div', { class: 'mes-categoria__nome' },
              el('span', { text: c.nome || 'sem categoria' }),
              el('span', { class: 'mes-categoria__qtd rotulo', text: ` (${c.quantidade})` }),
              el('span', { class: 'valor', text: ' ' + dinheiro.formatar(c.total_centavos) })),
            el('div', { class: 'barra' }, preenchido)));
        }
      }
      painel.append(categorias);

      // --- sinais ---
      const sinais = el('section', { class: 'cartao mes-sinais', 'aria-label': 'Sinais de alerta' },
        el('h2', { class: 'titulo-secao', text: 'sinais' }));
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
