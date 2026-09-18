// Tela de login: um formulario, um CTA. O roteador ja oculta header e nav.
import { svg } from '../app.js';

// Path do icone "wallet" do Lucide (stroke 1.75), so usado aqui.
const LOGO = `<svg class="login__logo" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false"><path d="M19 7V4a1 1 0 0 0-1-1H5a2 2 0 0 0 0 4h15a1 1 0 0 1 1 1v4h-3a2 2 0 0 0 0 4h3a1 1 0 0 0 1-1v-2a1 1 0 0 0-1-1"/><path d="M3 5v14a2 2 0 0 0 2 2h15a1 1 0 0 0 1-1v-4"/></svg>`;

const MODELO = `
<section class="login">
  <div class="login__cartao cartao">
    <div class="login__marca">
      ${LOGO}
      <h1 class="login__titulo">Caixa</h1>
      <p class="login__sub rotulo">Entre para ver o seu mês.</p>
    </div>

    <form class="login__form" novalidate>
      <div class="login__erro" id="login-erro" role="alert"></div>

      <div class="campo">
        <label for="login-usuario">Usuário</label>
        <input id="login-usuario" name="usuario" type="text" autocomplete="username"
               autocapitalize="none" autocorrect="off" spellcheck="false" inputmode="text"
               required aria-describedby="login-usuario-erro">
        <p class="campo__erro" id="login-usuario-erro" aria-live="polite"></p>
      </div>

      <div class="campo">
        <label for="login-senha">Senha</label>
        <div class="login__senha">
          <input id="login-senha" name="senha" type="password" autocomplete="current-password"
                 required aria-describedby="login-senha-erro login-senha-caps">
          <button type="button" class="botao botao--fantasma login__mostrar"
                  aria-pressed="false" aria-controls="login-senha">Mostrar</button>
        </div>
        <p class="login__caps rotulo" id="login-senha-caps" aria-live="polite"></p>
        <p class="campo__erro" id="login-senha-erro" aria-live="polite"></p>
      </div>

      <button type="submit" class="botao botao--primario login__cta">
        <span class="login__cta-icone" aria-hidden="true"></span>
        <span class="login__cta-rotulo">Entrar</span>
      </button>
    </form>
  </div>
</section>`;

const MENSAGENS = {
  401: 'usuário ou senha inválidos',
  429: 'muitas tentativas, aguarde um minuto',
};

// Garante o CSS da tela uma unica vez, resolvido a partir deste modulo
// (funciona seja qual for o prefixo em que o app estiver montado).
function garantirEstilo() {
  const href = new URL('./login.css', import.meta.url).href;
  if (document.querySelector(`link[href="${href}"]`)) return;
  const link = document.createElement('link');
  link.rel = 'stylesheet';
  link.href = href;
  document.head.appendChild(link);
}

// Login e o unico endpoint em que 401 e resposta normal, nao "sessao caiu".
// O cliente `api` reage a 401 navegando para /login, o que remontaria esta
// tela e apagaria a mensagem de erro; por isso o POST vai direto pelo fetch.
async function entrar(usuario, senha, signal) {
  const resposta = await fetch('/api/sessao', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify({ usuario, senha }),
    credentials: 'same-origin',
    signal,
  });
  if (resposta.ok) return;

  let mensagem = MENSAGENS[resposta.status];
  if (!mensagem) {
    try {
      const corpo = await resposta.json();
      if (corpo && typeof corpo.erro === 'string') mensagem = corpo.erro;
    } catch {
      // corpo vazio ou nao-JSON: cai na mensagem generica
    }
  }
  throw new Error(mensagem || 'não foi possível entrar, tente novamente');
}

export default {
  titulo: 'Entrar',

  async montar(raiz, ctx) {
    garantirEstilo();
    raiz.innerHTML = MODELO;

    const form = raiz.querySelector('.login__form');
    const erroForm = raiz.querySelector('#login-erro');
    const usuario = raiz.querySelector('#login-usuario');
    const senha = raiz.querySelector('#login-senha');
    const erroUsuario = raiz.querySelector('#login-usuario-erro');
    const erroSenha = raiz.querySelector('#login-senha-erro');
    const caps = raiz.querySelector('#login-senha-caps');
    const mostrar = raiz.querySelector('.login__mostrar');
    const cta = raiz.querySelector('.login__cta');
    const ctaIcone = raiz.querySelector('.login__cta-icone');
    const ctaRotulo = raiz.querySelector('.login__cta-rotulo');

    let controlador = null;

    function definirErroCampo(campo, elErro, texto) {
      elErro.textContent = texto;
      if (texto) campo.setAttribute('aria-invalid', 'true');
      else campo.removeAttribute('aria-invalid');
    }

    function definirErroForm(texto) {
      erroForm.replaceChildren();
      if (!texto) return;
      const icone = document.createElement('span');
      icone.className = 'login__erro-icone';
      icone.setAttribute('aria-hidden', 'true');
      icone.innerHTML = svg('alerta');
      erroForm.append(icone, document.createTextNode(texto));
    }

    function definirCarregando(ativo) {
      cta.disabled = ativo;
      form.setAttribute('aria-busy', String(ativo));
      ctaRotulo.textContent = ativo ? 'Entrando…' : 'Entrar';
      ctaIcone.innerHTML = ativo ? svg('carregando') : '';
      ctaIcone.classList.toggle('login__girar', ativo);
    }

    function validar() {
      const u = usuario.value.trim();
      const s = senha.value;
      definirErroCampo(usuario, erroUsuario, u ? '' : 'Informe o usuário.');
      definirErroCampo(senha, erroSenha, s ? '' : 'Informe a senha.');
      if (!u) usuario.focus();
      else if (!s) senha.focus();
      return Boolean(u && s);
    }

    async function aoEnviar(evento) {
      evento.preventDefault();
      if (cta.disabled) return;
      definirErroForm('');
      if (!validar()) return;

      controlador = new AbortController();
      definirCarregando(true);
      try {
        await entrar(usuario.value.trim(), senha.value, controlador.signal);
        senha.value = '';
        ctx.navegar('/mes', { substituir: true });
      } catch (erro) {
        if (erro && erro.name === 'AbortError') return;
        definirCarregando(false);
        const semRede = erro instanceof TypeError;
        definirErroForm(semRede ? 'sem conexão com o servidor' : erro.message);
        senha.focus();
        senha.select();
      } finally {
        controlador = null;
      }
    }

    function alternarSenha() {
      const visivel = mostrar.getAttribute('aria-pressed') === 'true';
      mostrar.setAttribute('aria-pressed', String(!visivel));
      mostrar.textContent = visivel ? 'Mostrar' : 'Ocultar';
      senha.type = visivel ? 'password' : 'text';
      senha.focus();
    }

    function avisarCaps(evento) {
      const ligado = typeof evento.getModifierState === 'function'
        && evento.getModifierState('CapsLock');
      caps.textContent = ligado ? 'Caps Lock está ativado.' : '';
    }

    function limparErroAoDigitar(evento) {
      const campo = evento.target;
      if (campo === usuario && usuario.value.trim()) definirErroCampo(usuario, erroUsuario, '');
      if (campo === senha && senha.value) definirErroCampo(senha, erroSenha, '');
    }

    form.addEventListener('submit', aoEnviar);
    mostrar.addEventListener('click', alternarSenha);
    senha.addEventListener('keyup', avisarCaps);
    senha.addEventListener('blur', () => { caps.textContent = ''; });
    form.addEventListener('input', limparErroAoDigitar);

    // Depois do foco que o roteador da em #conteudo: cai direto no campo.
    const quadro = requestAnimationFrame(() => usuario.focus());

    return function desmontar() {
      cancelAnimationFrame(quadro);
      if (controlador) controlador.abort();
      senha.value = '';
    };
  },
};
