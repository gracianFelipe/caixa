// Nucleo do PWA do Caixa: cliente de API, dinheiro em centavos inteiros,
// roteador por history, toasts, WebSocket com reconexao e icones SVG.
// Sem dependencias, sem build, sem inline (CSP default-src 'self').

// --- erro e cliente da API ---------------------------------------------

export class ErroAPI extends Error {
  constructor(status, mensagem) {
    super(mensagem);
    this.status = status;
    this.mensagem = mensagem;
  }
}

async function chamar(metodo, caminho, corpo) {
  const opcoes = { method: metodo, headers: {} };
  if (corpo !== undefined) {
    opcoes.headers['Content-Type'] = 'application/json';
    opcoes.body = JSON.stringify(corpo);
  }
  const resposta = await fetch('/api' + caminho, opcoes);

  if (resposta.status === 401 && caminho !== '/sessao') {
    navegar('/login', { substituir: true });
    throw new ErroAPI(401, 'sessao expirada');
  }
  if (resposta.status === 204) return null;

  let dados = null;
  try { dados = await resposta.json(); } catch { /* corpo vazio ou nao-JSON */ }
  if (!resposta.ok) {
    throw new ErroAPI(resposta.status, (dados && dados.erro) || 'erro inesperado');
  }
  return dados;
}

export const api = {
  get: (caminho) => chamar('GET', caminho),
  post: (caminho, corpo) => chamar('POST', caminho, corpo),
  put: (caminho, corpo) => chamar('PUT', caminho, corpo),
  del: (caminho) => chamar('DELETE', caminho),
};

// --- dinheiro: centavos inteiros, nunca float ---------------------------

export const dinheiro = {
  // 123456 -> "R$ 1.234,56"; -4790 -> "-R$ 47,90"; 7 -> "R$ 0,07".
  formatar(centavos) {
    const negativo = centavos < 0;
    let digitos = String(negativo ? -centavos : centavos).padStart(3, '0');
    const inteiros = digitos.slice(0, -2);
    const decimais = digitos.slice(-2);
    let agrupado = '';
    for (let i = 0; i < inteiros.length; i++) {
      const doFim = inteiros.length - i;
      agrupado += inteiros[i];
      if (doFim > 1 && (doFim - 1) % 3 === 0) agrupado += '.';
    }
    return (negativo ? '-' : '') + 'R$ ' + agrupado + ',' + decimais;
  },

  // Mesmas regras do dominio Go: virgula decimal com EXATAMENTE 2 casas
  // ("47,9" e ambiguo e falha), ponto so como milhar em grupos de 3.
  analisar(texto) {
    let s = String(texto ?? '').trim();
    if (s === '') throw new Error('informe um valor');
    let negativo = false;
    if (s.startsWith('-')) { negativo = true; s = s.slice(1).trim(); }
    else if (s.startsWith('+')) { s = s.slice(1).trim(); }
    if (s.startsWith('R$')) s = s.slice(2).trim();
    if (s.startsWith('-')) { if (negativo) throw new Error('valor invalido'); negativo = true; s = s.slice(1).trim(); }

    const virgula = s.indexOf(',');
    let inteiro = s, decimal = '00';
    if (virgula >= 0) {
      inteiro = s.slice(0, virgula);
      decimal = s.slice(virgula + 1);
      if (decimal.includes(',') || decimal.length !== 2 || /\D/.test(decimal)) {
        throw new Error('use duas casas depois da virgula (ex.: 47,90)');
      }
    }
    const grupos = inteiro.split('.');
    for (let i = 0; i < grupos.length; i++) {
      const g = grupos[i];
      if (g === '' || /\D/.test(g)) throw new Error('valor invalido');
      if (i > 0 && g.length !== 3) throw new Error('agrupamento de milhar invalido');
      if (i === 0 && grupos.length > 1 && g.length > 3) throw new Error('agrupamento de milhar invalido');
    }
    const digitos = grupos.join('') + decimal;
    if (digitos.length > 15) throw new Error('valor grande demais');
    const centavos = parseInt(digitos, 10); // <= 15 digitos: seguro em Number
    return negativo ? -centavos : centavos;
  },
};

// --- competencia AAAA-MM -------------------------------------------------

const MESES = ['janeiro', 'fevereiro', 'março', 'abril', 'maio', 'junho',
  'julho', 'agosto', 'setembro', 'outubro', 'novembro', 'dezembro'];

export const competencia = {
  atual() {
    const d = new Date();
    return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0');
  },
  valida(c) { return /^\d{4}-(0[1-9]|1[0-2])$/.test(c); },
  anterior(c) {
    let [ano, mes] = c.split('-').map(Number);
    mes--; if (mes === 0) { mes = 12; ano--; }
    return ano + '-' + String(mes).padStart(2, '0');
  },
  proxima(c) {
    let [ano, mes] = c.split('-').map(Number);
    mes++; if (mes === 13) { mes = 1; ano++; }
    return ano + '-' + String(mes).padStart(2, '0');
  },
  rotulo(c) {
    const [ano, mes] = c.split('-').map(Number);
    return MESES[mes - 1] + ' de ' + ano;
  },
};

// --- icones (paths do Lucide, stroke 1.75) -------------------------------

const ICONES = {
  mes: '<path d="M8 2v4M16 2v4M3 10h18M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z"/>',
  lista: '<path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01"/>',
  relatorio: '<path d="M3 3v18h18M18 17V9M13 17V5M8 17v-3"/>',
  orcamento: '<path d="M19 5c-1.5 0-2.8 1.4-3 2-3.5-1.5-11-.3-11 5 0 1.8 0 3 2 4.5V20h4v-2h3v2h4v-4c1-.5 1.7-1 2-2h2v-4h-2c0-1-.5-1.5-1-2V5z"/>',
  mais: '<path d="M12 5v14M5 12h14"/>',
  sair: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9"/>',
  esquerda: '<path d="M15 18l-6-6 6-6"/>',
  direita: '<path d="M9 18l6-6-6-6"/>',
  alerta: '<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0zM12 9v4M12 17h.01"/>',
  aviso: '<circle cx="12" cy="12" r="10"/><path d="M12 8v4M12 16h.01"/>',
  ok: '<path d="M22 11.1V12a10 10 0 1 1-5.9-9.1M22 4 12 14l-3-3"/>',
  x: '<path d="M18 6 6 18M6 6l12 12"/>',
  editar: '<path d="M17 3a2.8 2.8 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5L17 3z"/>',
  carregando: '<path d="M21 12a9 9 0 1 1-6.2-8.6"/>',
};

export function svg(nome) {
  const caminho = ICONES[nome] || ICONES.aviso;
  return '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" ' +
    'stroke="currentColor" stroke-width="1.75" stroke-linecap="round" ' +
    'stroke-linejoin="round" aria-hidden="true">' + caminho + '</svg>';
}

// --- toast ----------------------------------------------------------------

let regiaoDeToast = null;

export function toast(mensagem, tipo = 'info') {
  if (!regiaoDeToast) return;
  const t = document.createElement('div');
  t.className = 'toast' + (tipo !== 'info' ? ' toast--' + tipo : '');
  t.textContent = mensagem;
  regiaoDeToast.append(t);
  setTimeout(() => t.remove(), 4000);
}

// --- ao vivo (WebSocket com reconexao) ------------------------------------

const assinantes = new Set();
let ws = null;
let recuoMs = 1000;
let aoVivoLigado = false;

function conectarAoVivo() {
  if (!aoVivoLigado || (ws && ws.readyState <= WebSocket.OPEN)) return;
  const esquema = location.protocol === 'https:' ? 'wss://' : 'ws://';
  ws = new WebSocket(esquema + location.host + '/api/ao-vivo');
  ws.onopen = () => { recuoMs = 1000; };
  ws.onmessage = (ev) => {
    let aviso;
    try { aviso = JSON.parse(ev.data); } catch { return; }
    for (const fn of assinantes) fn(aviso);
  };
  ws.onclose = () => {
    ws = null;
    if (!aoVivoLigado) return;
    setTimeout(conectarAoVivo, recuoMs);
    recuoMs = Math.min(recuoMs * 2, 30000);
  };
}

function ligarAoVivo() { aoVivoLigado = true; conectarAoVivo(); }
function desligarAoVivo() {
  aoVivoLigado = false;
  if (ws) { ws.close(); ws = null; }
}

export const aoVivo = {
  assinar(fn) {
    assinantes.add(fn);
    return () => assinantes.delete(fn);
  },
};

// --- roteador ---------------------------------------------------------------

const ROTAS = {
  '/login': () => import('./telas/login.js'),
  '/': () => import('./telas/mes.js'),
  '/mes': () => import('./telas/mes.js'),
  '/lancamentos': () => import('./telas/lancamentos.js'),
  '/relatorio': () => import('./telas/relatorio.js'),
  '/orcamentos': () => import('./telas/orcamentos.js'),
};

const NAV = [
  { rota: '/mes', rotulo: 'Mês', icone: 'mes' },
  { rota: '/lancamentos', rotulo: 'Lançamentos', icone: 'lista' },
  { rota: '/relatorio', rotulo: 'Relatório', icone: 'relatorio' },
  { rota: '/orcamentos', rotulo: 'Orçamentos', icone: 'orcamento' },
];

let desmontarTelaAtual = null;
let elementos = null; // {cabecalho, titulo, seletorRotulo, conteudo, nav}

export function navegar(caminho, { substituir = false } = {}) {
  const alvo = caminho.startsWith('/') ? caminho : '/' + caminho;
  if (substituir) history.replaceState(null, '', alvo);
  else history.pushState(null, '', alvo);
  renderizar();
}

function competenciaDaURL() {
  const c = new URLSearchParams(location.search).get('competencia');
  return c && competencia.valida(c) ? c : competencia.atual();
}

function definirCompetencia(c) {
  const url = new URL(location.href);
  url.searchParams.set('competencia', c);
  history.replaceState(null, '', url);
  renderizar();
}

async function renderizar() {
  const rota = ROTAS[location.pathname] ? location.pathname : '/mes';
  const ehLogin = rota === '/login';

  if (typeof desmontarTelaAtual === 'function') {
    try { desmontarTelaAtual(); } catch { /* tela ja se foi */ }
    desmontarTelaAtual = null;
  }

  elementos.cabecalho.classList.toggle('oculto', ehLogin);
  elementos.nav.classList.toggle('oculto', ehLogin);
  if (ehLogin) desligarAoVivo(); else ligarAoVivo();

  for (const botao of elementos.nav.querySelectorAll('[data-rota]')) {
    if (botao.dataset.rota === (rota === '/' ? '/mes' : rota)) {
      botao.setAttribute('aria-current', 'page');
    } else {
      botao.removeAttribute('aria-current');
    }
  }

  const comp = competenciaDaURL();
  elementos.seletorRotulo.textContent = competencia.rotulo(comp);

  const modulo = await ROTAS[rota]();
  const tela = modulo.default;
  document.title = tela.titulo + ' — Caixa';
  elementos.titulo.textContent = tela.titulo;

  elementos.conteudo.replaceChildren();
  const ctx = {
    competencia: comp,
    definirCompetencia,
    params: new URLSearchParams(location.search),
    navegar,
    toast,
  };
  desmontarTelaAtual = await tela.montar(elementos.conteudo, ctx) || null;
  elementos.conteudo.focus({ preventScroll: true });
}

// --- montagem do shell -------------------------------------------------------

function montarShell() {
  const corpo = document.body;

  const cabecalho = document.createElement('header');
  cabecalho.className = 'cabecalho';
  const titulo = document.createElement('h1');
  titulo.className = 'cabecalho__titulo';

  const seletor = document.createElement('div');
  seletor.className = 'seletor-competencia';
  const btAnterior = botaoDeSeta('esquerda', 'Mês anterior', () => definirCompetencia(competencia.anterior(competenciaDaURL())));
  const rotulo = document.createElement('span');
  rotulo.className = 'seletor-competencia__rotulo';
  const btProxima = botaoDeSeta('direita', 'Próximo mês', () => definirCompetencia(competencia.proxima(competenciaDaURL())));
  seletor.append(btAnterior, rotulo, btProxima);
  cabecalho.append(titulo, seletor);

  const conteudo = document.createElement('main');
  conteudo.id = 'conteudo';
  conteudo.tabIndex = -1;

  const nav = document.createElement('nav');
  nav.className = 'nav-inferior';
  nav.setAttribute('aria-label', 'Principal');
  for (const item of NAV) {
    const botao = document.createElement('a');
    botao.className = 'nav-inferior__item';
    botao.href = item.rota;
    botao.dataset.rota = item.rota;
    botao.innerHTML = svg(item.icone);
    const texto = document.createElement('span');
    texto.textContent = item.rotulo;
    botao.append(texto);
    botao.addEventListener('click', (ev) => {
      ev.preventDefault();
      navegar(item.rota);
    });
    nav.append(botao);
  }

  regiaoDeToast = document.createElement('div');
  regiaoDeToast.className = 'toast-regiao';
  regiaoDeToast.setAttribute('aria-live', 'polite');

  corpo.append(cabecalho, conteudo, nav, regiaoDeToast);
  elementos = { cabecalho, titulo, seletorRotulo: rotulo, conteudo, nav };
}

function botaoDeSeta(icone, rotulo, aoClicar) {
  const b = document.createElement('button');
  b.type = 'button';
  b.className = 'botao botao--fantasma';
  b.setAttribute('aria-label', rotulo);
  b.innerHTML = svg(icone);
  b.addEventListener('click', aoClicar);
  return b;
}

async function iniciar() {
  montarShell();
  window.addEventListener('popstate', renderizar);

  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('/sw.js').catch(() => { /* offline segue sem sw */ });
  }

  try {
    await api.get('/sessao');
    if (location.pathname === '/login') {
      navegar('/mes', { substituir: true });
      return;
    }
  } catch {
    navegar('/login', { substituir: true });
    return;
  }
  renderizar();
}

iniciar();
