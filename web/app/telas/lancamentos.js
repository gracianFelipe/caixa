// Tela "Lançamentos": lista do mês agrupada por dia, filtro rápido e
// troca de categoria inline. Só lê /api/lancamentos, /api/categorias e
// escreve em PUT /api/lancamentos/{id}/categoria.
import { api, dinheiro, competencia, aoVivo, svg } from '../app.js';

const MEIOS = {
  pix: 'Pix',
  credito: 'Crédito',
  debito: 'Débito',
  boleto: 'Boleto',
  dinheiro: 'Dinheiro',
  transferencia: 'Transferência',
};

const FILTROS = [
  { id: 'todos', rotulo: 'Todos', aplica: () => true },
  { id: 'sem-categoria', rotulo: 'Sem categoria', aplica: (l) => l.categoria_id == null },
  { id: 'provisorios', rotulo: 'Provisórios', aplica: (l) => l.situacao === 'provisorio' },
];

const fmtDia = new Intl.DateTimeFormat('pt-BR', { day: 'numeric', month: 'long' });
const fmtHora = new Intl.DateTimeFormat('pt-BR', { hour: '2-digit', minute: '2-digit' });

// Extrato CSV nao traz hora: o lancamento fica a meia-noite local. Exibir
// "00:00" seria inventar uma precisao que o dado nao tem — nesses casos a
// hora simplesmente nao aparece.
function horaDe(instante) {
  const d = new Date(instante);
  if (d.getHours() === 0 && d.getMinutes() === 0) return '';
  return ' · ' + fmtHora.format(d);
}

// Preferências que sobrevivem à remontagem (troca de competência, ao vivo),
// mas não são dados financeiros: ficam em memória.
let filtroAtual = 'todos';

// Deep link: /lancamentos?filtro=sem-categoria (o chip de pendentes do Mes).
function filtroDaURL(ctx) {
  const f = ctx.params.get('filtro');
  if (f && FILTROS.some((x) => x.id === f)) filtroAtual = f;
}
let categoriasCache = null;

// --- utilitários de DOM (sem innerHTML com dado do usuário) -----------------

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

// O markup do ícone vem do próprio app.js (confiável), por isso innerHTML aqui.
function icone(nome) {
  const s = el('span', { class: 'lancamentos__icone', 'aria-hidden': 'true' });
  s.innerHTML = svg(nome);
  return s;
}

function garantirCss() {
  const href = new URL('./lancamentos.css', import.meta.url).href;
  if ([...document.styleSheets].some((s) => s.href === href)) return;
  if (document.querySelector(`link[href="${href}"]`)) return;
  document.head.append(el('link', { rel: 'stylesheet', href }));
}

const pad = (n) => String(n).padStart(2, '0');

// Chave de agrupamento no fuso local: o mesmo instante RFC3339 pode cair em
// dias diferentes em UTC e no Brasil.
function chaveDia(iso) {
  const d = new Date(iso);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

function agruparPorDia(lancamentos) {
  const grupos = new Map();
  // A API devolve ascendente; a lista mostra o mais recente primeiro.
  for (let i = lancamentos.length - 1; i >= 0; i--) {
    const l = lancamentos[i];
    const chave = chaveDia(l.ocorrido_em);
    if (!grupos.has(chave)) grupos.set(chave, { rotulo: fmtDia.format(new Date(l.ocorrido_em)), itens: [] });
    grupos.get(chave).itens.push(l);
  }
  return [...grupos.values()];
}

function plural(n, um, varios) {
  return `${n} ${n === 1 ? um : varios}`;
}

// --- tela -------------------------------------------------------------------

export default {
  titulo: 'Lançamentos',

  async montar(raiz, ctx) {
    filtroDaURL(ctx);
    garantirCss();

    const estado = {
      ativo: true,
      carregando: true,
      erro: null,
      lancamentos: [],
      categorias: categoriasCache ?? [],
      abertoId: null, // lançamento com o seletor de categoria expandido
      salvandoId: null,
    };

    const status = el('p', { class: 'oculto-visualmente', role: 'status', 'aria-live': 'polite' });
    const filtro = el('div', { class: 'filtro-segmentado', role: 'group', 'aria-label': 'Filtrar lançamentos' });
    const corpo = el('div', { class: 'lancamentos__corpo' });
    const tela = el('section', { class: 'lancamentos', 'aria-label': 'Lançamentos do mês' }, filtro, status, corpo);
    raiz.replaceChildren(tela);

    // ---- filtro segmentado ----
    function renderizarFiltro() {
      filtro.replaceChildren(
        ...FILTROS.map((f) => {
          const n = f.id === 'todos' ? estado.lancamentos.length : estado.lancamentos.filter(f.aplica).length;
          return el(
            'button',
            {
              type: 'button',
              class: 'filtro-segmentado__botao',
              'aria-pressed': String(f.id === filtroAtual),
              onclick: () => {
                if (filtroAtual === f.id) return;
                filtroAtual = f.id;
                estado.abertoId = null;
                renderizarFiltro();
                renderizarLista();
              },
            },
            el('span', { text: f.rotulo }),
            estado.carregando ? null : el('span', { class: 'filtro-segmentado__contagem', text: String(n), 'aria-label': `${n}` }),
          );
        }),
      );
    }

    // ---- carregamento ----
    async function carregar({ silencioso = false } = {}) {
      if (!silencioso) {
        estado.carregando = true;
        estado.erro = null;
        renderizarFiltro();
        renderizarLista();
      }
      try {
        const [lancamentos, categorias] = await Promise.all([
          api.get(`/lancamentos?competencia=${encodeURIComponent(ctx.competencia)}`),
          categoriasCache ? Promise.resolve(categoriasCache) : api.get('/categorias'),
        ]);
        if (!estado.ativo) return;
        categoriasCache = categorias ?? [];
        estado.categorias = categoriasCache;
        estado.lancamentos = lancamentos ?? [];
        estado.erro = null;
      } catch (e) {
        if (!estado.ativo) return;
        // Erro de rede não tem `mensagem`; ErroAPI tem.
        estado.erro = e?.mensagem || 'Não foi possível carregar os lançamentos.';
        if (silencioso) ctx.toast(estado.erro, 'erro');
      } finally {
        if (estado.ativo) {
          estado.carregando = false;
          renderizarFiltro();
          renderizarLista();
        }
      }
    }

    // ---- lista ----
    function renderizarLista() {
      if (estado.carregando) {
        corpo.replaceChildren(esqueleto());
        status.textContent = 'Carregando lançamentos';
        return;
      }
      if (estado.erro) {
        corpo.replaceChildren(
          vazio('Algo deu errado', estado.erro, 'Tentar de novo', () => carregar()),
        );
        status.textContent = estado.erro;
        return;
      }

      const f = FILTROS.find((x) => x.id === filtroAtual) ?? FILTROS[0];
      const visiveis = estado.lancamentos.filter(f.aplica);
      const semCategoria = estado.lancamentos.filter((l) => l.categoria_id == null).length;
      status.textContent = `${plural(visiveis.length, 'lançamento', 'lançamentos')}${
        filtroAtual === 'todos' && semCategoria ? `, ${semCategoria} sem categoria` : ''
      }`;

      if (visiveis.length === 0) {
        if (estado.lancamentos.length === 0) {
          corpo.replaceChildren(
            vazio('Nenhum lançamento', `Nada registrado em ${competencia.rotulo(ctx.competencia)}.`),
          );
        } else {
          corpo.replaceChildren(
            vazio(
              filtroAtual === 'provisorios' ? 'Nenhum provisório' : 'Tudo categorizado',
              filtroAtual === 'provisorios'
                ? 'Todos os lançamentos do mês já foram confirmados.'
                : 'Todos os lançamentos do mês têm categoria.',
              'Ver todos',
              () => {
                filtroAtual = 'todos';
                renderizarFiltro();
                renderizarLista();
              },
            ),
          );
        }
        return;
      }

      corpo.replaceChildren(
        ...agruparPorDia(visiveis).map((g) =>
          el(
            'section',
            { class: 'lancamentos__dia' },
            el('h2', { class: 'lancamentos__dia-titulo rotulo', text: g.rotulo }),
            el('ul', { class: 'lista cartao lancamentos__lista' }, ...g.itens.map(item)),
          ),
        ),
      );
    }

    function esqueleto() {
      const linha = () =>
        el(
          'li',
          { class: 'lista__item lancamentos__item lancamentos__item--esqueleto' },
          el('div', { class: 'lancamentos__linha' },
            el('div', { class: 'lancamentos__principal' },
              el('span', { class: 'esqueleto lancamentos__esq-texto' }),
              el('span', { class: 'esqueleto lancamentos__esq-meta' })),
            el('span', { class: 'esqueleto lancamentos__esq-valor' })),
          el('div', { class: 'lancamentos__badges' }, el('span', { class: 'esqueleto lancamentos__esq-chip' })),
        );
      return el(
        'div',
        { class: 'lancamentos__corpo', 'aria-hidden': 'true' },
        el('section', { class: 'lancamentos__dia' },
          el('span', { class: 'esqueleto lancamentos__esq-dia' }),
          el('ul', { class: 'lista cartao lancamentos__lista' }, linha(), linha(), linha())),
        el('section', { class: 'lancamentos__dia' },
          el('span', { class: 'esqueleto lancamentos__esq-dia' }),
          el('ul', { class: 'lista cartao lancamentos__lista' }, linha(), linha())),
      );
    }

    function vazio(titulo, texto, rotuloAcao, acao) {
      return el(
        'div',
        { class: 'vazio' },
        el('h2', { class: 'vazio__titulo', text: titulo }),
        el('p', { class: 'vazio__texto', text: texto }),
        rotuloAcao ? el('button', { type: 'button', class: 'botao botao--fantasma', text: rotuloAcao, onclick: acao }) : null,
      );
    }

    // ---- item ----
    function nomeCategoria(id) {
      return estado.categorias.find((c) => c.id === id)?.nome ?? `Categoria ${id}`;
    }

    function item(l) {
      const descartado = l.situacao === 'descartado';
      const entrada = l.valor_centavos > 0;
      const semCategoria = l.categoria_id == null;
      const aberto = estado.abertoId === l.id;
      const idSeletor = `lanc-seletor-${l.id}`;

      const rotuloCategoria = semCategoria ? 'sem categoria' : nomeCategoria(l.categoria_id);
      const chipCategoria = semCategoria
        ? el('span', { class: 'sinal sinal--aviso' }, icone('aviso'), el('span', { text: rotuloCategoria }))
        : el('span', { class: 'lancamentos__chip lancamentos__chip--categoria', text: rotuloCategoria });

      const botaoCategoria = el(
        'button',
        {
          type: 'button',
          class: 'lancamentos__categoria',
          'aria-expanded': String(aberto),
          'aria-controls': idSeletor,
          'aria-label': descartado
            ? `Categoria: ${rotuloCategoria}`
            : `Categoria: ${rotuloCategoria}. Toque para alterar`,
          disabled: descartado,
          onclick: () => alternarSeletor(l.id),
        },
        chipCategoria,
        descartado ? null : icone('editar'),
      );

      const li = el(
        'li',
        {
          class: [
            'lista__item',
            'lancamentos__item',
            descartado && 'lancamentos__item--descartado',
            aberto && 'lancamentos__item--aberto',
          ].filter(Boolean).join(' '),
          'data-id': String(l.id),
        },
        el(
          'div',
          { class: 'lancamentos__linha' },
          el(
            'div',
            { class: 'lancamentos__principal' },
            el('span', { class: 'lancamentos__contraparte', text: l.contraparte || '(sem descrição)' }),
            el('span', {
              class: 'lancamentos__meta rotulo',
              text: `${MEIOS[l.meio] ?? l.meio}${horaDe(l.ocorrido_em)}`,
            }),
          ),
          el('span', {
            class: `valor ${entrada ? 'valor--entrada' : 'valor--saida'}`,
            text: dinheiro.formatar(l.valor_centavos),
          }),
        ),
        el(
          'div',
          { class: 'lancamentos__badges' },
          botaoCategoria,
          l.situacao === 'provisorio'
            ? el('span', { class: 'lancamentos__chip lancamentos__chip--provisorio', text: 'provisório' })
            : null,
          descartado ? el('span', { class: 'lancamentos__chip lancamentos__chip--descartado', text: 'descartado' }) : null,
        ),
        seletor(l, idSeletor, aberto),
      );
      return li;
    }

    function seletor(l, id, aberto) {
      const salvando = estado.salvandoId === l.id;
      const opcoes = estado.categorias.map((c) =>
        el(
          'button',
          {
            type: 'button',
            class: 'botao botao--fantasma lancamentos__opcao',
            'aria-pressed': String(c.id === l.categoria_id),
            disabled: salvando,
            onclick: () => trocarCategoria(l, c.id),
          },
          el('span', { text: c.nome }),
          c.id === l.categoria_id ? icone('ok') : null,
        ),
      );
      return el(
        'div',
        {
          id,
          class: 'lancamentos__seletor',
          role: 'group',
          'aria-label': `Escolher categoria para ${l.contraparte || 'lançamento'}`,
          hidden: !aberto,
          'aria-busy': String(salvando),
        },
        opcoes.length
          ? el('div', { class: 'lancamentos__opcoes' }, ...opcoes)
          : el('p', { class: 'rotulo', text: 'Nenhuma categoria cadastrada.' }),
        el(
          'div',
          { class: 'lancamentos__seletor-rodape' },
          salvando ? el('span', { class: 'rotulo lancamentos__salvando' }, icone('carregando'), el('span', { text: 'Salvando…' })) : null,
          el('button', {
            type: 'button',
            class: 'botao botao--fantasma',
            text: 'Cancelar',
            disabled: salvando,
            onclick: () => fecharSeletor({ devolverFoco: true }),
          }),
        ),
      );
    }

    // Reconstrói só o <li> do lançamento, mantendo o resto da lista intacto.
    function atualizarItem(l) {
      const atual = corpo.querySelector(`li[data-id="${CSS.escape(String(l.id))}"]`);
      if (!atual) return null;
      const novo = item(l);
      atual.replaceWith(novo);
      return novo;
    }

    function alternarSeletor(id) {
      if (estado.salvandoId) return;
      if (estado.abertoId === id) {
        fecharSeletor({ devolverFoco: true });
        return;
      }
      const anterior = estado.abertoId;
      estado.abertoId = id;
      if (anterior != null) {
        const la = estado.lancamentos.find((x) => x.id === anterior);
        if (la) atualizarItem(la);
      }
      const l = estado.lancamentos.find((x) => x.id === id);
      const li = l && atualizarItem(l);
      li?.querySelector('.lancamentos__opcao:not([disabled])')?.focus();
    }

    function fecharSeletor({ devolverFoco = false } = {}) {
      const id = estado.abertoId;
      if (id == null) return;
      estado.abertoId = null;
      const l = estado.lancamentos.find((x) => x.id === id);
      const li = l && atualizarItem(l);
      if (devolverFoco) li?.querySelector('.lancamentos__categoria')?.focus();
    }

    async function trocarCategoria(l, categoriaId) {
      if (estado.salvandoId) return;
      if (categoriaId === l.categoria_id) {
        fecharSeletor({ devolverFoco: true });
        return;
      }
      estado.salvandoId = l.id;
      atualizarItem(l);
      try {
        const atualizado = await api.put(`/lancamentos/${encodeURIComponent(l.id)}/categoria`, { categoria_id: categoriaId });
        if (!estado.ativo) return;
        const i = estado.lancamentos.findIndex((x) => x.id === l.id);
        // Se a resposta vier vazia, aplica localmente o que o servidor faria.
        const novo = atualizado ?? { ...l, categoria_id: categoriaId, categoria_origem: 'manual' };
        if (i >= 0) estado.lancamentos[i] = novo;
        estado.salvandoId = null;
        estado.abertoId = null;
        const li = atualizarItem(novo);
        li?.querySelector('.lancamentos__categoria')?.focus();
        renderizarFiltro();
        ctx.toast(`Categoria alterada para ${nomeCategoria(novo.categoria_id)}`, 'sucesso');
      } catch (e) {
        if (!estado.ativo) return;
        estado.salvandoId = null;
        atualizarItem(l);
        ctx.toast(e?.mensagem || 'Não foi possível alterar a categoria.', 'erro');
      }
    }

    function aoTeclar(ev) {
      if (ev.key === 'Escape' && estado.abertoId != null) {
        ev.preventDefault();
        fecharSeletor({ devolverFoco: true });
      }
    }
    tela.addEventListener('keydown', aoTeclar);

    // ---- ao vivo: várias mensagens seguidas viram uma recarga só ----
    let temporizador = null;
    const cancelarAoVivo = aoVivo.assinar((msg) => {
      if (msg?.tipo !== 'lancamento_criado') return;
      clearTimeout(temporizador);
      temporizador = setTimeout(() => {
        temporizador = null;
        if (estado.ativo && !estado.salvandoId) carregar({ silencioso: true });
      }, 500);
    });

    await carregar();

    return function desmontar() {
      estado.ativo = false;
      clearTimeout(temporizador);
      cancelarAoVivo();
      tela.removeEventListener('keydown', aoTeclar);
      tela.remove();
    };
  },
};
