// Service worker do Caixa: cache-first para o shell, network-only para /api.
// Dado financeiro NUNCA entra em cache do navegador; o que fica offline e a
// casca do app, que abre e avisa que esta sem rede.

const CACHE = 'caixa-v1';

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

  ev.respondWith(
    caches.match(ev.request).then((doCache) => doCache || fetch(ev.request))
  );
});
