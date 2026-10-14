/* SYNCO — utilidades comunes a todas las pantallas. */
'use strict';

let E = null;            // estado completo de la simulación
let RED = null;          // direcciones para compartir
let CONECTADOS = [];
const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => Array.from(r.querySelectorAll(s));

function guardar(k, v) { try { localStorage.setItem('synco.' + k, JSON.stringify(v)); } catch (e) {} }
function recuperar(k, d) { try { const v = localStorage.getItem('synco.' + k); return v == null ? d : JSON.parse(v); } catch (e) { return d; } }

function pid() {
  let id = recuperar('pid', null);
  if (!id) { id = 'p' + Math.random().toString(36).slice(2, 10); guardar('pid', id); }
  return id;
}

function claveDocente() {
  const q = new URLSearchParams(location.search).get('clave');
  if (q) guardar('clave', q);
  return q || recuperar('clave', '');
}

async function api(ruta, datos) {
  const r = await fetch(ruta, {
    method: 'POST', headers: { 'Content-Type': 'application/json', 'X-Clave': claveDocente() },
    body: JSON.stringify(datos || {}),
  });
  const j = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(j.error || ('Error ' + r.status));
  return j;
}

async function obtener(ruta) {
  const r = await fetch(ruta, { headers: { 'X-Clave': claveDocente() } });
  const j = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(j.error || ('Error ' + r.status));
  return j;
}

function el(tag, props, ...hijos) {
  const n = document.createElement(tag);
  if (props) for (const [k, v] of Object.entries(props)) {
    if (v == null || v === false) continue;
    if (k === 'class') n.className = v;
    else if (k === 'html') n.innerHTML = v;
    else if (k.startsWith('on')) n.addEventListener(k.slice(2), v);
    else if (k === 'style' && typeof v === 'object') Object.assign(n.style, v);
    else n.setAttribute(k, v === true ? '' : v);
  }
  for (const h of hijos.flat()) if (h != null && h !== false) n.append(h.nodeType ? h : document.createTextNode(String(h)));
  return n;
}

function esc(s) { return String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])); }

function num(x, d = 0) {
  if (x == null || isNaN(x)) return '—';
  return Number(x).toLocaleString('es-CL', { minimumFractionDigits: d, maximumFractionDigits: d });
}

const MESES = ['enero', 'febrero', 'marzo', 'abril', 'mayo', 'junio', 'julio', 'agosto', 'septiembre', 'octubre', 'noviembre', 'diciembre'];
const DIAS = ['domingo', 'lunes', 'martes', 'miércoles', 'jueves', 'viernes', 'sábado'];
function fechaLarga(iso) {
  if (!iso) return '';
  const [a, m, d] = iso.split('-').map(Number);
  const f = new Date(Date.UTC(a, m - 1, d));
  return `${DIAS[f.getUTCDay()]} ${d} de ${MESES[m - 1]} de ${a}`;
}

function aviso(texto, tipo) {
  let c = $('#avisos');
  if (!c) { c = el('div', { id: 'avisos', role: 'status', 'aria-live': 'polite' }); document.body.append(c); }
  const a = el('div', { class: 'aviso' + (tipo === 'error' ? ' error' : '') }, texto);
  c.append(a);
  setTimeout(() => a.remove(), tipo === 'error' ? 7000 : 5000);
}

async function intentar(f) {
  try { return await f(); } catch (e) { aviso(e.message, 'error'); }
}

// ---------- logo y formas ----------

function logoSVG() {
  return `<svg viewBox="0 0 40 40" aria-hidden="true"><polygon points="20,2 36,11 36,29 20,38 4,29 4,11" fill="none" stroke="#f07f22" stroke-width="3"/>
  <circle cx="20" cy="20" r="3.2" fill="#f1e9dc"/><g fill="#f07f22">${[0, 1, 2, 3, 4, 5].map(i => {
    const a = Math.PI / 3 * i - Math.PI / 2; return `<circle cx="${20 + 10 * Math.cos(a)}" cy="${20 + 10 * Math.sin(a)}" r="2.4"/>`;
  }).join('')}</g></svg>`;
}

const NOMBRES_FORMA = ['círculo', 'cuadrado', 'triángulo', 'rombo', 'cruz'];
function formaSVG(i, color = 'currentColor', relleno = true) {
  const f = relleno ? color : 'none';
  const t = `stroke="${color}" stroke-width="2.5" fill="${f}"`;
  const d = [
    `<circle cx="12" cy="12" r="9" ${t}/>`,
    `<rect x="3.5" y="3.5" width="17" height="17" ${t}/>`,
    `<polygon points="12,2.5 21.5,20.5 2.5,20.5" ${t}/>`,
    `<polygon points="12,2 22,12 12,22 2,12" ${t}/>`,
    `<path d="M9 2.5h6v6.5h6.5v6H15v6.5H9V15H2.5V9H9z" ${t}/>`,
  ][i];
  return `<svg class="forma" viewBox="0 0 24 24" role="img" aria-label="${NOMBRES_FORMA[i]}">${d}</svg>`;
}
function comboHTML(combo, color) { return `<span class="combo">${(combo || []).map(i => formaSVG(i, color)).join('')}</span>`; }

// ---------- barra superior con el reloj ----------

function barra(titulo) {
  const b = el('header', { class: 'barra' },
    el('a', { class: 'logo', href: '/', html: logoSVG() + `<b>SYNCO</b><span>${esc(titulo || '')}</span>` }),
    el('div', { class: 'reloj', id: 'reloj' }, '…'),
    el('span', { class: 'punto', id: 'conexion', title: 'conexión' }),
    el('div', { class: 'progreso', id: 'progreso' }));
  document.body.prepend(b);
  pintarReloj();
}

function pintarReloj() {
  const r = E && E.reloj, n = $('#reloj');
  if (!r || !n) return;
  const esp = r.esperando && r.esperando.length ? `<div class="esperando">esperando a: ${esc(r.esperando.slice(0, 4).join(', '))}${r.esperando.length > 4 ? '…' : ''}</div>` : '';
  n.innerHTML = `<div><span class="hora">${r.hora}</span> <span class="larga">· ${fechaLarga(r.fecha)}</span><span class="corta"></span></div><div class="fase">${r.pausa ? '⏸ en pausa · ' : ''}${esc(r.tituloFase)} · día ${r.dia}</div>${esp}`;
  const p = $('#progreso');
  if (p) p.style.width = `${(r.minuto - 480 + (r.minuto < 480 ? 1440 : 0)) / 1440 * 100}%`;
  const c = $('#conexion');
  if (c) c.className = 'punto ' + (r.pausa ? 'pausa' : 'vivo');
}

// ---------- conexión en tiempo real ----------

let manejadores = [];
function alEvento(f) { manejadores.push(f); }

async function recargar() {
  const j = await obtener('/api/estado');
  E = j.estado; RED = j.red; CONECTADOS = j.conectados || [];
  E.telex = E.telex || []; E.senales = E.senales || []; E.eventos = E.eventos || [];
  manejadores.forEach(f => f('estado', E));
  pintarReloj();
}

function conectar(rol, ref) {
  let es, reintento = 1000;
  const abrir = () => {
    es = new EventSource(`/api/eventos?rol=${encodeURIComponent(rol)}&ref=${encodeURIComponent(ref || '')}&id=${pid()}`);
    es.onmessage = ev => {
      reintento = 1000;
      let m; try { m = JSON.parse(ev.data); } catch (e) { return; }
      aplicar(m.t, m.d);
    };
    es.onerror = () => {
      const c = $('#conexion'); if (c) c.className = 'punto caido';
      es.close(); setTimeout(() => { abrir(); recargar().catch(() => {}); }, reintento);
      reintento = Math.min(reintento * 2, 10000);
    };
  };
  return recargar().then(abrir);
}

function aplicar(t, d) {
  if (!E) return;
  switch (t) {
    case 'hola': break;
    case 'estado': recargar(); return;
    case 'reloj': E.reloj = d; pintarReloj(); break;
    case 'fabricas': E.fabricas = d; break;
    case 'senales': E.senales = d || []; break;
    case 'senal': { const i = E.senales.findIndex(s => s.id === d.id); if (i >= 0) E.senales[i] = d; else E.senales.push(d); break; }
    case 'telex': E.telex.push(d); if (E.telex.length > 200) E.telex.splice(0, E.telex.length - 200); break;
    case 'telexFin': { const x = E.telex.find(m => m.id === d.id); if (x) x.estado = d.estado; break; }
    case 'indices': E.indices = d.indices; E.total = d.total; E.porSector = d.porSector; break;
    case 'sala': E.sala = d; break;
    case 'pueblo': E.pueblo = Object.assign({}, E.pueblo, d); break;
    case 'politica': E.pueblo = Object.assign({}, E.pueblo, { politica: d.politica, desde: d.desde, serie: [] }); break;
    case 'config': E.config = d; break;
    case 'camiones': E.camiones = d; break;
    case 'ecom': E.ecom = d; break;
    case 'bitacora': E.bitacora.push(d); if (E.bitacora.length > 300) E.bitacora.shift(); break;
    case 'evento': E.eventos.push(d); break;
    case 'conectados': CONECTADOS = d || []; break;
  }
  manejadores.forEach(f => f(t, d));
}

// ---------- utilidades de dominio ----------

function fabrica(id) { return E && E.fabricas.find(f => f.id === id); }
function sector(id) { return E && E.sectores.find(s => s.id === id); }
function indice(c) { return E && E.indices.find(i => i.codigo === c); }
const NIVELES = ['', 'Empresa', 'Comité sectorial', 'CORFO', 'Sala de Operaciones'];

function urlCompartir(ruta) {
  const base = (RED && RED.urls && RED.urls[0]) || location.origin;
  return base + ruta;
}
function qrImg(texto, tam = 160) {
  return el('img', { src: '/api/qr?texto=' + encodeURIComponent(texto), width: tam, height: tam, alt: 'Código QR: ' + texto, style: { background: '#fff', borderRadius: '6px', display: 'block' } });
}

// ---------- ITA2 (para dibujar la cinta perforada) ----------

const ITA2 = (() => {
  const L = { A: '11000', B: '10011', C: '01110', D: '10010', E: '10000', F: '10110', G: '01011', H: '00101', I: '01100', J: '11010', K: '11110', L: '01001', M: '00111', N: '00110', O: '00011', P: '01101', Q: '11101', R: '01010', S: '10100', T: '00001', U: '11100', V: '01111', W: '11001', X: '10111', Y: '10101', Z: '10001' };
  const C = { '-': '11000', '?': '10011', ':': '01110', '3': '10000', '8': '01100', '(': '11110', ')': '01001', '.': '00111', ',': '00110', '9': '00011', '0': '01101', '1': '11101', '4': '01010', "'": '10100', '5': '00001', '7': '11100', '=': '01111', '2': '11001', '/': '10111', '6': '10101', '+': '10001' };
  function normalizar(s) {
    return s.normalize('NFD').replace(/[̀-ͯ]/g, '').toUpperCase().replace(/\r/g, '').replace(/[^A-Z0-9 \n\-?:().,'=/+]/g, '');
  }
  function codificar(s) {
    s = normalizar(s);
    const out = []; let caso = '';
    for (const ch of s) {
      if (ch === '\n') { out.push({ b: '00010', s: 'CR' }, { b: '01000', s: 'LF' }); continue; }
      if (ch === ' ') { out.push({ b: '00100', s: '␣' }); continue; }
      if (L[ch]) { if (caso !== 'L') { out.push({ b: '11111', s: 'LTRS' }); caso = 'L'; } out.push({ b: L[ch], s: ch }); continue; }
      if (C[ch]) { if (caso !== 'C') { out.push({ b: '11011', s: 'FIGS' }); caso = 'C'; } out.push({ b: C[ch], s: ch }); }
    }
    return out;
  }
  return { codificar, normalizar };
})();

function dibujarCinta(cont, tramas, max = 400) {
  cont.innerHTML = '';
  const t = tramas.slice(-max);
  const frag = document.createDocumentFragment();
  for (const x of t) {
    const c = el('div', { class: 'col5', title: `${x.s} = ${x.b}` });
    for (let i = 0; i < 5; i++) {
      if (i === 3) c.append(el('div', { class: 'arrastre' }));
      c.append(el('div', { class: 'agujero' + (x.b[i] === '1' ? ' si' : '') }));
    }
    c.append(el('div', { class: 'sim' }, x.s.length > 1 ? x.s[0] : x.s));
    frag.append(c);
  }
  cont.append(frag);
  cont.scrollLeft = cont.scrollWidth;
}

// ---------- sonido de teleimpresor (WebAudio, sin archivos) ----------

const Sonido = (() => {
  let ctx = null, activo = recuperar('sonido', true);
  function iniciar() { if (!ctx) { try { ctx = new (window.AudioContext || window.webkitAudioContext)(); } catch (e) {} } if (ctx && ctx.state === 'suspended') ctx.resume(); }
  function golpe() {
    if (!activo || !ctx) return;
    const n = ctx.createBufferSource(), b = ctx.createBuffer(1, 600, ctx.sampleRate), d = b.getChannelData(0);
    for (let i = 0; i < d.length; i++) d[i] = (Math.random() * 2 - 1) * Math.exp(-i / 90);
    n.buffer = b;
    const g = ctx.createGain(); g.gain.value = 0.18;
    const f = ctx.createBiquadFilter(); f.type = 'bandpass'; f.frequency.value = 1800 + Math.random() * 600;
    n.connect(f).connect(g).connect(ctx.destination); n.start();
  }
  function campana() {
    if (!activo || !ctx) return;
    const o = ctx.createOscillator(), g = ctx.createGain();
    o.frequency.value = 1320; g.gain.setValueAtTime(0.25, ctx.currentTime); g.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 1.2);
    o.connect(g).connect(ctx.destination); o.start(); o.stop(ctx.currentTime + 1.2);
  }
  document.addEventListener('pointerdown', iniciar, { once: false });
  return { golpe, campana, get activo() { return activo; }, alternar() { activo = !activo; guardar('sonido', activo); return activo; } };
})();

// Imprime un texto carácter por carácter a la velocidad del télex.
function imprimirTelex(destino, texto, msPorCaracter, alTerminar, clase) {
  const span = el('span', { class: clase || '' });
  destino.append(span);
  let i = 0, ultimo = performance.now(), acumulado = 0;
  const cursor = el('span', { class: 'cursor' }, ' ');
  destino.append(cursor);
  function paso(t) {
    acumulado += t - ultimo; ultimo = t;
    let n = Math.floor(acumulado / msPorCaracter);
    if (n > 0) {
      acumulado -= n * msPorCaracter;
      const trozo = texto.slice(i, i + n); i += n;
      span.textContent += trozo;
      if (trozo.trim()) Sonido.golpe();
      destino.scrollTop = destino.scrollHeight;
    }
    if (i < texto.length) requestAnimationFrame(paso);
    else { cursor.remove(); alTerminar && alTerminar(); }
  }
  requestAnimationFrame(paso);
}

// ---------- gráficos ----------

const NS = 'http://www.w3.org/2000/svg';
function svg(tag, attrs, ...hijos) {
  const n = document.createElementNS(NS, tag);
  for (const [k, v] of Object.entries(attrs || {})) if (v != null) n.setAttribute(k, v);
  for (const h of hijos.flat()) if (h != null) n.append(h.nodeType ? h : document.createTextNode(String(h)));
  return n;
}

function minilinea(valores, ancho = 120, alto = 28, color = '#f07f22') {
  const s = svg('svg', { viewBox: `0 0 ${ancho} ${alto}`, width: ancho, height: alto, 'aria-hidden': 'true' });
  if (!valores || valores.length < 2) return s;
  const lo = Math.min(...valores), hi = Math.max(...valores), r = (hi - lo) || 1;
  const pts = valores.map((v, i) => `${(i / (valores.length - 1) * (ancho - 2) + 1).toFixed(1)},${(alto - 2 - (v - lo) / r * (alto - 4)).toFixed(1)}`).join(' ');
  s.append(svg('polyline', { points: pts, fill: 'none', stroke: color, 'stroke-width': 1.8, 'stroke-linejoin': 'round' }));
  return s;
}

// Serie de P con banda taxonómica y pronóstico de Harrison-Stevens.
function graficoP(cont, historia, banda, opciones = {}) {
  const W = opciones.ancho || 640, H = opciones.alto || 260, m = { i: 46, d: 14, s: 18, b: 28 };
  const datos = historia.slice(-(opciones.ultimos || 90));
  const s = svg('svg', { viewBox: `0 0 ${W} ${H}`, width: '100%', role: 'img', 'aria-label': 'Productividad P en el tiempo con banda taxonómica' });
  if (!datos.length) { cont.replaceChildren(s); return; }
  const vals = datos.map(d => d.p).concat([banda.inf, banda.sup]).concat(datos.map(d => d.pron).filter(x => x));
  let lo = Math.max(0, Math.min(...vals) - 0.05), hi = Math.min(1.02, Math.max(...vals) + 0.05);
  const x = i => m.i + i / Math.max(1, datos.length) * (W - m.i - m.d);
  const y = v => m.s + (1 - (v - lo) / (hi - lo)) * (H - m.s - m.b);
  const c = opciones.color || '#f07f22';
  s.append(svg('rect', { x: m.i, y: y(banda.sup), width: W - m.i - m.d, height: Math.max(1, y(banda.inf) - y(banda.sup)), fill: 'rgba(93,181,107,.12)', stroke: 'rgba(93,181,107,.45)', 'stroke-dasharray': '4 4' }));
  s.append(svg('line', { x1: m.i, x2: W - m.d, y1: y(banda.med), y2: y(banda.med), stroke: 'rgba(93,181,107,.5)' }));
  for (let k = 0; k <= 4; k++) {
    const v = lo + (hi - lo) * k / 4;
    s.append(svg('text', { x: m.i - 6, y: y(v) + 4, 'text-anchor': 'end', 'font-size': 11, fill: '#a99c8b' }, v.toFixed(2)));
  }
  const ini = datos.findIndex(d => !d.previo);
  if (ini > 0) {
    s.append(svg('line', { x1: x(ini - 0.5), x2: x(ini - 0.5), y1: m.s, y2: H - m.b, stroke: '#786c5e', 'stroke-dasharray': '2 3' }));
    s.append(svg('text', { x: x(ini - 0.5) + 4, y: m.s + 10, 'font-size': 10, fill: '#a99c8b' }, 'inicio de la sesión'));
  }
  const pron = datos.map((d, i) => d.pron ? `${x(i + 1).toFixed(1)},${y(d.pron).toFixed(1)}` : null).filter(Boolean);
  s.append(svg('polyline', { points: pron.join(' '), fill: 'none', stroke: '#57a3ef', 'stroke-width': 1.4, 'stroke-dasharray': '5 3', opacity: .85 }));
  s.append(svg('polyline', { points: datos.map((d, i) => `${x(i).toFixed(1)},${y(d.p).toFixed(1)}`).join(' '), fill: 'none', stroke: c, 'stroke-width': 2.2, 'stroke-linejoin': 'round' }));
  datos.forEach((d, i) => {
    const cl = d.clas || {};
    const anom = cl.fueraBanda || cl.escalonAyer || cl.transAyer || cl.pendiente;
    if (anom && !d.previo) s.append(svg('circle', { cx: x(i), cy: y(d.p), r: 4.5, fill: '#e8432f', stroke: '#fff', 'stroke-width': 1 }));
  });
  const ult = datos[datos.length - 1];
  s.append(svg('circle', { cx: x(datos.length - 1), cy: y(ult.p), r: 4, fill: c }));
  s.append(svg('text', { x: m.i, y: H - 8, 'font-size': 11, fill: '#a99c8b' }, `${datos[0].fecha.slice(0, 2)}-${datos[0].fecha.slice(2, 4)}-${datos[0].fecha.slice(4)}`));
  s.append(svg('text', { x: W - m.d, y: H - 8, 'font-size': 11, fill: '#a99c8b', 'text-anchor': 'end' }, `${ult.fecha.slice(0, 2)}-${ult.fecha.slice(2, 4)}-${ult.fecha.slice(4)}`));
  cont.replaceChildren(s);
}

function probabilidadesHTML(prob) {
  const nombres = ['sin cambio', 'escalón', 'pendiente', 'transitorio'];
  const colores = ['#5db56b', '#e8432f', '#f3c64f', '#57a3ef'];
  return `<div style="display:flex;height:14px;border-radius:4px;overflow:hidden;background:#000">${prob.map((p, i) => `<div title="${nombres[i]} ${(p * 100).toFixed(1)}%" style="width:${(p * 100).toFixed(2)}%;background:${colores[i]}"></div>`).join('')}</div>
  <div class="fila" style="gap:10px;margin-top:4px;font-size:.78rem">${prob.map((p, i) => `<span><span class="punto" style="background:${colores[i]}"></span> ${nombres[i]} ${(p * 100).toFixed(0)}%</span>`).join('')}</div>`;
}
