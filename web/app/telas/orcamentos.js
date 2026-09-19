// Tela de orçamentos: uma linha por categoria com barra gasto/limite e
// formulário inline para definir o limite (padrão ou só da competência).
import { api, ErroAPI, dinheiro, competencia as comp, aoVivo, svg } from '../app.js';

// A folha da tela é carregada sob demanda; a URL sai de import.meta.url para
// não depender de onde o app está montado nem da rota atual (/orcamentos).
const FOLHA = new URL('./orcamentos.css', import.meta.url).href;

const LIMIAR_AVISO = 80;
const LIMIAR_PERIGO = 100;

export default {
  titulo: 'Orçamentos',

  async montar(raiz, ctx) {
    garantirFolha();

    let ativo = true;
    let formularioAberto = null; // { fechar() } da linha em edição

    const secao = el('section', 'orcamentos');
    raiz.replaceChildren(secao);

    async function carregar() {
      fecharFormulario();
      secao.replaceChildren(esqueleto());
      let linhas;
      try {
        linhas = await api.get('/orcamentos?competencia=' + encodeURIComponent(ctx.competencia));
      } catch (erro) {
        if (!ativo) return;
        secao.replaceChildren(estadoErro(erro, carregar));
        return;
      }
      if (!ativo) return;
      if (!Array.isArray(linhas) || linhas.length === 0) {
        secao.replaceChildren(estadoVazio());
        return;
      }
      secao.replaceChildren(resumo(linhas), lista(linhas));
    }

    function fecharFormulario() {
      if (formularioAberto) {
        formularioAberto.fechar({ focar: false });
        formularioAberto = null;
      }
    }

    function lista(linhas) {
      const ul = el('ul', 'lista orcamentos__lista');
      for (const linha of linhas) ul.append(itemOrcamento(linha));
      return ul;
    }

    function itemOrcamento(o) {
      const gasto = Math.abs(Number(o.gasto_centavos) || 0);
      const temLimite = o.limite_centavos !== null && o.limite_centavos !== undefined;
      const limite = temLimite ? Number(o.limite_centavos) : 0;
      const pct = temLimite && limite > 0 ? Math.floor((gasto * 100) / limite) : 0;

      const li = el('li', 'lista__item orcamento');
      if (temLimite && pct >= LIMIAR_PERIGO) li.classList.add('orcamento--perigo');
      else if (temLimite && pct >= LIMIAR_AVISO) li.classList.add('orcamento--aviso');

      // Cabeçalho: nome, chip "do mês", botão editar.
      const cabecalho = el('div', 'orcamento__cabecalho');
      const nome = el('span', 'orcamento__nome', o.nome);
      cabecalho.append(nome);
      if (o.especifico) {
        const chip = el('span', 'orcamento__chip', 'do mês');
        chip.title = 'Limite válido só para ' + comp.rotulo(ctx.competencia);
        cabecalho.append(chip);
      }
      const botaoEditar = el('button', 'botao botao--fantasma orcamento__editar');
      botaoEditar.type = 'button';
      botaoEditar.setAttribute('aria-label', 'Editar limite de ' + o.nome);
      botaoEditar.append(icone('editar'));
      cabecalho.append(botaoEditar);
      li.append(cabecalho);

      // Barra de progresso (largura via --pct, nunca style="width").
      if (temLimite) {
        const barra = el('div', 'barra orcamento__barra');
        barra.setAttribute('role', 'progressbar');
        barra.setAttribute('aria-valuemin', '0');
        barra.setAttribute('aria-valuemax', '100');
        barra.setAttribute('aria-valuenow', String(Math.min(pct, 100)));
        barra.setAttribute('aria-valuetext', pct + '% do limite de ' + o.nome);
        const preenchido = el('div', 'barra__preenchido');
        preenchido.style.setProperty('--pct', Math.min(pct, 100) + '%');
        barra.append(preenchido);
        li.append(barra);
      }

      // Texto: "R$ gasto de R$ limite" ou "sem limite", mais sinal quando passa dos limiares.
      const rodape = el('div', 'orcamento__rodape');
      const texto = el('span', 'orcamento__texto');
      if (temLimite) {
        texto.append(
          el('span', 'valor valor--saida', dinheiro.formatar(gasto)),
          document.createTextNode(' de '),
          el('span', 'valor orcamento__limite', dinheiro.formatar(limite)),
        );
        const pctEl = el('span', 'orcamento__pct', pct + '%');
        rodape.append(texto, pctEl);
      } else {
        texto.append(
          el('span', 'valor valor--saida', dinheiro.formatar(gasto)),
          document.createTextNode(' · '),
          el('span', 'rotulo', 'sem limite'),
        );
        rodape.append(texto);
      }
      li.append(rodape);

      if (temLimite && pct >= LIMIAR_PERIGO) {
        li.append(sinal('perigo', 'alerta', 'Estourou o limite'));
      } else if (temLimite && pct >= LIMIAR_AVISO) {
        li.append(sinal('aviso', 'aviso', 'Perto do limite'));
      }

      botaoEditar.addEventListener('click', () => {
        if (formularioAberto && formularioAberto.li === li) {
          fecharFormulario();
          return;
        }
        fecharFormulario();
        formularioAberto = abrirFormulario(li, o, botaoEditar);
      });

      return li;
    }

    function abrirFormulario(li, o, botaoEditar) {
      const idBase = 'orc-' + o.categoria_id;
      const form = el('form', 'orcamento__form');
      form.noValidate = true;
      form.setAttribute('aria-label', 'Limite de ' + o.nome);

      // Campo do valor.
      const campo = el('div', 'campo');
      const label = el('label', null, 'Limite mensal (R$)');
      label.htmlFor = idBase + '-valor';
      const input = el('input');
      input.id = idBase + '-valor';
      input.name = 'limite';
      input.type = 'text';
      input.inputMode = 'decimal';
      input.autocomplete = 'off';
      input.placeholder = '0,00';
      input.required = true;
      if (o.limite_centavos !== null && o.limite_centavos !== undefined) {
        input.value = dinheiro.formatar(Number(o.limite_centavos));
      }
      const ajuda = el('p', 'campo__ajuda', 'Use vírgula para centavos, ex.: 800,00');
      ajuda.id = idBase + '-ajuda';
      const erro = el('p', 'campo__erro');
      erro.id = idBase + '-erro';
      erro.setAttribute('aria-live', 'polite');
      input.setAttribute('aria-describedby', ajuda.id + ' ' + erro.id);
      campo.append(label, input, ajuda, erro);

      // Escopo: padrão (todo mês) ou só esta competência.
      const escopo = el('fieldset', 'orcamento__escopo');
      escopo.append(el('legend', 'rotulo', 'Vale para'));
      const nomeRadio = idBase + '-escopo';
      const opcaoPadrao = radio(nomeRadio, 'padrao', 'Padrão (todo mês)', !o.especifico);
      const opcaoMes = radio(nomeRadio, 'mes', 'Só ' + comp.rotulo(ctx.competencia), !!o.especifico);
      escopo.append(opcaoPadrao.rotulo, opcaoMes.rotulo);

      // Ações.
      const acoes = el('div', 'orcamento__acoes');
      const salvar = el('button', 'botao botao--primario', 'Salvar');
      salvar.type = 'submit';
      const cancelar = el('button', 'botao botao--fantasma', 'Cancelar');
      cancelar.type = 'button';
      acoes.append(cancelar, salvar);

      form.append(campo, escopo, acoes);
      li.append(form);
      li.classList.add('orcamento--editando');
      botaoEditar.setAttribute('aria-expanded', 'true');

      function validar() {
        erro.textContent = '';
        input.removeAttribute('aria-invalid');
        const texto = input.value.trim();
        if (texto === '') {
          return falhar('Informe o limite.');
        }
        let centavos;
        try {
          centavos = dinheiro.analisar(texto);
        } catch (e) {
          return falhar(e && e.message ? e.message : 'Valor inválido.');
        }
        if (!Number.isInteger(centavos) || centavos <= 0) {
          return falhar('O limite precisa ser maior que zero.');
        }
        return centavos;
      }

      function falhar(mensagem) {
        erro.textContent = mensagem;
        input.setAttribute('aria-invalid', 'true');
        return null;
      }

      input.addEventListener('blur', () => {
        if (input.value.trim() !== '') validar();
      });
      input.addEventListener('input', () => {
        if (erro.textContent) {
          erro.textContent = '';
          input.removeAttribute('aria-invalid');
        }
      });

      function fechar({ focar = true } = {}) {
        form.remove();
        li.classList.remove('orcamento--editando');
        botaoEditar.setAttribute('aria-expanded', 'false');
        if (focar) botaoEditar.focus();
      }

      cancelar.addEventListener('click', () => {
        fechar();
        formularioAberto = null;
      });
      form.addEventListener('keydown', (ev) => {
        if (ev.key === 'Escape') {
          ev.preventDefault();
          fechar();
          formularioAberto = null;
        }
      });

      form.addEventListener('submit', async (ev) => {
        ev.preventDefault();
        const centavos = validar();
        if (centavos === null) {
          input.focus();
          return;
        }
        const soEsteMes = opcaoMes.input.checked;
        salvar.disabled = true;
        cancelar.disabled = true;
        try {
          await api.put('/orcamentos', {
            categoria_id: o.categoria_id,
            limite_centavos: centavos,
            competencia: soEsteMes ? ctx.competencia : null,
          });
        } catch (e) {
          if (!ativo) return;
          salvar.disabled = false;
          cancelar.disabled = false;
          const mensagem = e instanceof ErroAPI && e.mensagem ? e.mensagem : 'Não foi possível salvar o limite.';
          falhar(mensagem);
          ctx.toast(mensagem, 'erro');
          input.focus();
          return;
        }
        if (!ativo) return;
        ctx.toast(
          soEsteMes
            ? 'Limite de ' + o.nome + ' salvo para ' + comp.rotulo(ctx.competencia) + '.'
            : 'Limite padrão de ' + o.nome + ' salvo.',
          'sucesso',
        );
        formularioAberto = null;
        await carregar();
      });

      input.focus();
      return { li, fechar };
    }

    // Lançamento novo muda o gasto; recarrega só se ninguém está editando,
    // para não perder o que o usuário digitou.
    const cancelarAoVivo = aoVivo.assinar((evento) => {
      if (!ativo || !evento || evento.tipo !== 'lancamento_criado') return;
      if (formularioAberto) return;
      carregar();
    });

    await carregar();

    return function desmontar() {
      ativo = false;
      cancelarAoVivo();
      formularioAberto = null;
    };
  },
};

// ---- Peças ------------------------------------------------------------------

function resumo(linhas) {
  const comLimite = linhas.filter((l) => l.limite_centavos !== null && l.limite_centavos !== undefined);
  const cartao = el('div', 'painel-hero orcamentos__resumo');
  if (comLimite.length === 0) {
    cartao.append(
      el('p', 'rotulo', 'Nenhum limite definido'),
      el('p', 'orcamentos__resumo-texto', 'Toque em editar numa categoria para definir o primeiro limite.'),
    );
    return cartao;
  }
  let gasto = 0;
  let limite = 0;
  for (const l of comLimite) {
    gasto += Math.abs(Number(l.gasto_centavos) || 0);
    limite += Number(l.limite_centavos) || 0;
  }
  const estouradas = comLimite.filter((l) => {
    const g = Math.abs(Number(l.gasto_centavos) || 0);
    return Number(l.limite_centavos) > 0 && Math.floor((g * 100) / Number(l.limite_centavos)) >= LIMIAR_PERIGO;
  }).length;

  cartao.append(el('p', 'rotulo', 'Gasto nas categorias com limite'));
  const linha = el('p', 'orcamentos__resumo-valor');
  linha.append(
    el('span', 'valor valor--destaque valor--saida', dinheiro.formatar(gasto)),
    el('span', 'rotulo', ' de ' + dinheiro.formatar(limite)),
  );
  cartao.append(linha);
  const detalhe =
    comLimite.length + (comLimite.length === 1 ? ' categoria com limite' : ' categorias com limite') +
    (estouradas > 0 ? ' · ' + estouradas + (estouradas === 1 ? ' estourada' : ' estouradas') : '');
  cartao.append(el('p', 'orcamentos__resumo-texto', detalhe));
  return cartao;
}

function esqueleto() {
  const ul = el('ul', 'lista orcamentos__lista');
  ul.setAttribute('aria-busy', 'true');
  ul.setAttribute('aria-label', 'Carregando orçamentos');
  for (let i = 0; i < 4; i++) {
    const li = el('li', 'lista__item orcamento orcamento--esqueleto');
    li.append(
      el('div', 'esqueleto orcamento__esq-nome'),
      el('div', 'esqueleto orcamento__esq-barra'),
      el('div', 'esqueleto orcamento__esq-texto'),
    );
    ul.append(li);
  }
  return ul;
}

function estadoVazio() {
  const caixa = el('div', 'vazio');
  caixa.append(
    el('h2', 'vazio__titulo', 'Nenhuma categoria ainda'),
    el('p', 'vazio__texto', 'Os limites aparecem aqui quando houver categorias cadastradas.'),
  );
  return caixa;
}

function estadoErro(erro, tentarDeNovo) {
  const caixa = el('div', 'vazio');
  caixa.setAttribute('role', 'alert');
  const mensagem = erro instanceof ErroAPI && erro.mensagem ? erro.mensagem : 'Não foi possível carregar os orçamentos.';
  const botao = el('button', 'botao botao--primario', 'Tentar de novo');
  botao.type = 'button';
  botao.addEventListener('click', tentarDeNovo);
  caixa.append(el('h2', 'vazio__titulo', 'Algo deu errado'), el('p', 'vazio__texto', mensagem), botao);
  return caixa;
}

function sinal(tipo, nomeIcone, texto) {
  const chip = el('span', 'sinal sinal--' + tipo + ' orcamento__sinal');
  chip.append(icone(nomeIcone), document.createTextNode(texto));
  return chip;
}

function radio(nome, valor, texto, marcado) {
  const rotulo = el('label', 'orcamento__opcao');
  const input = el('input');
  input.type = 'radio';
  input.name = nome;
  input.value = valor;
  input.checked = marcado;
  rotulo.append(input, el('span', null, texto));
  return { rotulo, input };
}

function icone(nome) {
  const span = el('span', 'icone');
  span.setAttribute('aria-hidden', 'true');
  span.innerHTML = svg(nome); // markup interno e fixo, nunca dado do usuário
  return span;
}

function el(tag, classes, texto) {
  const e = document.createElement(tag);
  if (classes) e.className = classes;
  if (texto !== undefined) e.textContent = texto;
  return e;
}

function garantirFolha() {
  if (document.querySelector('link[data-tela="orcamentos"]')) return;
  const link = document.createElement('link');
  link.rel = 'stylesheet';
  link.href = FOLHA;
  link.dataset.tela = 'orcamentos';
  document.head.append(link);
}
