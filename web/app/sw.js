// Service worker do Caixa: cache-first para o shell, network-only para /api.
// Dado financeiro NUNCA entra em cache do navegador; o que fica offline e a
// casca do app, que abre e avisa que esta sem rede.

// A versao entra no NOME do cache: trocar esta linha invalida o shell
// inteiro. Sem isso, um deploy novo nunca chega a quem ja abriu o app.
const CACHE = 'caixa-v2';

const SHELL = [
  '/',
  '/index.html',
  '/app.css',
  '/app.js',
  '/manifest.webmanifest',
  '/icones/icone.svg',
  '/icones/icone-maskable.svg',
  '/telas/login.js', '/telas/login.css',
  '/telas/mes.js', '/telas/mes.css',
  '/telas/lancamentos.js', '/telas/lancamentos.css',
  '/telas/relatorio.js', '/telas/relatorio.css',
  '/telas/orcamentos.js', '/telas/orcamentos.css',
];

self.addEventListener('install', (ev) => {
  ev.waitUntil(
    caches.open(CACHE).then((c) => c.addAll(SHELL)).then(() => self.skipWaiting())
  );
});

self.addEventListener('activate', (ev) => {
  ev.waitUntil(
    caches.keys()
      .then((nomes) => Promise.all(nomes.filter((n) => n !== CACHE).map((n) => caches.delete(n))))
      .then(() => self.clients.claim())
  );
});

self.addEventListener('fetch', (ev) => {
  const url = new URL(ev.request.url);

  // /api nunca passa pelo cache: nem leitura nem escrita.
  if (url.pathname.startsWith('/api/') || url.pathname === '/saude') return;
  if (ev.request.method !== 'GET') return;

  // Navegacao offline: devolve o shell.
  if (ev.request.mode === 'navigate') {
    ev.respondWith(
      fetch(ev.request).catch(() => caches.match('/index.html'))
    );
    return;
  }

  // stale-while-revalidate: responde do cache (rapido e funciona offline) e,
  // em paralelo, busca da rede para atualizar o cache. Cache-first puro
  // servia CSS/JS velhos para sempre — foi assim que uma tela continuou com
  // a paleta antiga depois do redesign.
  ev.respondWith(
    caches.open(CACHE).then(async (cache) => {
      const doCache = await cache.match(ev.request);
      const daRede = fetch(ev.request)
        .then((resposta) => {
          if (resposta && resposta.ok && resposta.type === 'basic') {
            cache.put(ev.request, resposta.clone());
          }
          return resposta;
        })
        .catch(() => doCache); // offline: fica com o que ja tinha
      return doCache || daRede;
    })
  );
});
