/* Chat Service — ban live cua chat-thread-demo.html
 *
 * Giao dien giu nguyen CSS/HTML cua demo, nhung moi du lieu lay tu API that:
 *   GET  /api/client/v1/chat/bootstrap               -> danh sach nhom + ho so + thong bao
 *   GET  /api/client/v1/chat/groups/:id/messages     -> lich su tin nhan (cursor)
 *   POST /api/client/v1/chat/groups/:id/messages     -> gui tin (du phong khi WS dut)
 *   POST /api/client/v1/chat/groups/:id/read         -> danh dau da xem
 *   GET  /api/client/v1/chat/messages/:id/replies    -> thread / tra loi
 *   POST /api/client/v1/chat/messages/:id/actions    -> bam nut tren card
 *   POST /api/client/v1/chat/forms/:formId/submit    -> tao yeu cau (CMS tao don)
 *   GET  /ws?token=...                               -> realtime
 *
 * Man hinh "Don hang" / "Tep" / "Work item" cua demo bi an: do CMS so huu, Chat khong co du lieu.
 */
(() => {
  'use strict';

  // ---------------------------------------------------------------- tien ich
  const $ = id => document.getElementById(id);
  const esc = v => String(v ?? '').replace(/[&<>"']/g, c =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const icon = name => `<svg class="icon" aria-hidden="true"><use href="#i-${name}"/></svg>`;
  const uuid = () => (crypto.randomUUID ? crypto.randomUUID()
    : 'c-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2));
  const initial = name => String(name || '?').trim().slice(0, 1).toUpperCase() || '?';
  const hhmm = d => new Date(d).toLocaleTimeString('vi-VN', { hour: '2-digit', minute: '2-digit' });
  const dayKey = d => new Date(d).toISOString().slice(0, 10);
  const dayLabel = d => {
    const today = dayKey(new Date()), key = dayKey(d);
    if (key === today) return 'Hôm nay';
    const y = new Date(Date.now() - 864e5);
    if (key === dayKey(y)) return 'Hôm qua';
    return new Date(d).toLocaleDateString('vi-VN', { day: '2-digit', month: '2-digit', year: 'numeric' });
  };
  const norm = s => String(s || '').toLowerCase().normalize('NFD').replace(/[̀-ͯ]/g, '');

  function toast(text, kind) {
    const el = $('toast');
    if (!el) return;
    el.textContent = text;
    el.className = 'toast show' + (kind === 'error' ? ' error' : '');
    clearTimeout(toast.timer);
    toast.timer = setTimeout(() => { el.className = 'toast'; }, 4200);
  }

  // ------------------------------------------------------------------- state
  const store = {
    get base() { return localStorage.getItem('chat_base') || location.origin; },
    set base(v) { localStorage.setItem('chat_base', v); },
    get token() { return localStorage.getItem('chat_token') || ''; },
    set token(v) { v ? localStorage.setItem('chat_token', v) : localStorage.removeItem('chat_token'); },
  };

  const S = {
    base: store.base,
    token: '',
    me: null,
    groups: [],            // [{id, chatId, name, source, unreadCount, members[], cms{}}]
    activeId: null,         // id noi bo cua nhom dang mo
    msgs: new Map(),        // groupId -> {items: [], pageInfo: {}, loaded: bool}
    thread: null,           // {rootId, root, items}
    replyTo: null,          // {place: 'group'|'thread', id, name, text}
    requestMode: false,
    notifications: [],
    unreadNotifications: 0,
    typing: new Map(),      // groupId -> Map(userId -> timeout)
    ws: null,
    wsTimer: null,
    wsTries: 0,
    pending: new Map(),     // client_msg_id -> {resolve, reject, timer}
  };

  const activeGroup = () => S.groups.find(g => g.id === S.activeId) || null;
  const groupKey = g => g && (g.chatId || g.id);
  const bucket = gid => {
    if (!S.msgs.has(gid)) S.msgs.set(gid, { items: [], pageInfo: {}, loaded: false });
    return S.msgs.get(gid);
  };

  // --------------------------------------------------------------- API client
  async function call(path, { method = 'GET', body } = {}) {
    const res = await fetch(S.base + path, {
      method,
      headers: {
        'Content-Type': 'application/json',
        ...(S.token ? { Authorization: 'Bearer ' + S.token } : {}),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    let json = null;
    try { json = await res.json(); } catch (_) { /* body rong */ }
    if (!res.ok || (json && typeof json.statusCode === 'number' && json.statusCode >= 400)) {
      const err = new Error((json && json.message) || res.statusText || 'Lỗi gọi API');
      err.code = (json && json.code) || String(res.status);
      err.status = res.status;
      throw err;
    }
    return json ? (json.element !== undefined ? json.element : json) : null;
  }

  const Api = {
    me: () => call('/api/client/v1/chat/auth/me'),
    bootstrap: () => call('/api/client/v1/chat/bootstrap'),
    refreshGroups: () => call('/api/client/v1/chat/groups/refresh', { method: 'POST' }),
    messages: (gid, q = {}) =>
      call(`/api/client/v1/chat/groups/${encodeURIComponent(gid)}/messages?` +
        new URLSearchParams(Object.entries(q).filter(([, v]) => v !== undefined && v !== ''))),
    send: (gid, body) =>
      call(`/api/client/v1/chat/groups/${encodeURIComponent(gid)}/messages`, { method: 'POST', body }),
    markRead: (gid, messageId) =>
      call(`/api/client/v1/chat/groups/${encodeURIComponent(gid)}/read`, { method: 'POST', body: { messageId } }),
    replies: (id, mode = 'thread') =>
      call(`/api/client/v1/chat/messages/${id}/replies?mode=${mode}&limit=100`),
    action: (id, action, values) =>
      call(`/api/client/v1/chat/messages/${id}/actions`, { method: 'POST', body: { action, values } }),
    submitForm: (formId, values, context) =>
      call(`/api/client/v1/chat/forms/${formId}/submit`, { method: 'POST', body: { values, context } }),
    notifications: () => call('/api/client/v1/chat/notifications?limit=50'),
    readNotifications: ids => call('/api/client/v1/chat/notifications/read', { method: 'POST', body: { ids } }),
  };

  /* REST tra camelCase, realtime (WS) tra snake_case cua domain.Message.
     Chuan hoa ve mot dang duy nhat de phan render chi phai biet mot hinh dang. */
  function normalize(m) {
    if (!m) return null;
    const sender = m.sender || null;
    return {
      id: m.id,
      conversationId: m.conversationId || m.conversation_id || null,
      type: m.type || 'text',
      senderType: m.senderType || m.sender_type || 'user',
      senderId: m.senderId || m.sender_id || null,
      text: m.text !== undefined ? m.text : (m.content || ''),
      createdAt: m.createdAt || m.created_at,
      clientMsgId: m.clientMsgId || m.client_msg_id || '',
      threadRootId: m.threadRootId || m.thread_id || null,
      replyToId: m.replyToId || m.reply_to_id || null,
      replyCount: m.replyCount || m.reply_count || 0,
      threadCount: m.threadCount || m.thread_count || 0,
      attachments: m.attachments || [],
      card: m.card || null,
      cardVersion: m.cardVersion || m.card_version || 0,
      actions: m.actions || [],
      bot: m.bot || m.bot_sender || null,
      metadata: m.metadata || null,
      pinned: m.pinned || false,
      sender: sender ? {
        id: sender.id,
        actorId: sender.actorId || sender.actor_id || '',
        name: sender.name || sender.display_name || 'Người dùng',
        avatarUrl: sender.avatarUrl || sender.avatar_url || '',
        role: sender.role || '',
        presence: sender.presence || '',
      } : null,
    };
  }

  function senderOf(m) {
    if (m.senderType === 'system') return { name: 'Hệ thống', initial: '•' };
    if (m.senderType === 'bot' || m.bot) {
      const name = (m.bot && (m.bot.name || m.bot.Name)) || 'Bot';
      return { name, initial: initial(name) };
    }
    if (m.sender) return { name: m.sender.name, initial: initial(m.sender.name), role: m.sender.role };
    const g = activeGroup();
    const mem = g && g.members.find(x => x.userId === m.senderId);
    if (mem) return { name: mem.name || 'Người dùng', initial: initial(mem.name) };
    return { name: 'Người dùng', initial: '?' };
  }

  const isMine = m => !!(S.me && m.senderId && m.senderId === S.me.id);

  // ------------------------------------------------------------------ card UI
  /* CMS so huu tu vung card. Chat chi loc visibleTo roi tra nguyen van, nen bo
     render nay di theo kieu "biet gi ve gi", khong biet thi hien thi tho de khong mat du lieu. */
  const TONE = { info: 'doing', success: 'done', warning: 'paused', danger: 'overdue', muted: 'reference' };

  function cardHeaderHTML(card) {
    const h = card.header || {};
    const st = h.status || {};
    const pill = st.text
      ? `<span class="status-pill ${TONE[st.tone] || 'new'}">${esc(st.text)}</span>` : '';
    const badge = h.code || h.kind
      ? `<span class="task-badge">${esc([h.kind, h.code].filter(Boolean).join(' · '))}</span>` : '';
    return `<div class="task-heading">${badge}${pill}</div>` +
      (h.title ? `<strong>${esc(h.title)}</strong>` : '');
  }

  function cardElementHTML(el, msgId) {
    if (el == null) return '';
    if (typeof el === 'string') return `<div class="message-text">${esc(el)}</div>`;
    if (Array.isArray(el)) return el.map(e => cardElementHTML(e, msgId)).join('');
    const type = String(el.type || '').toLowerCase();

    switch (type) {
      case 'text':
      case 'paragraph':
        return `<div class="message-text">${esc(el.text || el.value || '')}</div>`;

      case 'note':
      case 'hint':
      case 'warning':
        return `<div class="card-note">${esc(el.text || el.value || '')}</div>`;

      case 'divider':
        return '<hr class="card-el">';

      case 'field':
      case 'fields':
      case 'facts': {
        const rows = el.items || el.fields || (el.label !== undefined ? [el] : []);
        if (!rows.length) return '';
        return `<dl class="card-fields">${rows.map(f =>
          `<dt>${esc(f.label || f.name || '')}</dt><dd>${esc(f.value ?? f.text ?? '')}</dd>`).join('')}</dl>`;
      }

      case 'button': {
        const action = el.action || el.id || '';
        const style = el.style === 'primary' ? 'primary-button'
          : el.style === 'danger' ? 'secondary-button' : 'subtle-button';
        const form = el.form || el.fields ? ` data-form='${esc(JSON.stringify(el.form || el.fields))}'` : '';
        const confirm = el.confirm ? ` data-confirm="${esc(el.confirm.text || el.confirm)}"` : '';
        return `<button type="button" class="${style} card-action" data-action="${esc(action)}"` +
          ` data-message="${esc(msgId)}"${form}${confirm}>${esc(el.text || el.label || action)}</button>`;
      }

      case 'actions':
      case 'row':
      case 'group':
      case 'section': {
        const kids = (el.elements || el.items || el.children || []).map(e => cardElementHTML(e, msgId)).join('');
        const cls = type === 'actions' || type === 'row' ? 'card-actions' : 'card-el';
        return kids ? `<div class="${cls}">${kids}</div>` : '';
      }

      case 'list': {
        const items = el.items || [];
        return items.length
          ? `<ul class="card-el">${items.map(i =>
              `<li>${esc(typeof i === 'string' ? i : (i.text || i.label || ''))}</li>`).join('')}</ul>`
          : '';
      }

      default: {
        // Loai chua biet: van hien ra chu de khong am tham mat noi dung.
        const kids = (el.elements || el.items || el.children);
        if (Array.isArray(kids)) return kids.map(e => cardElementHTML(e, msgId)).join('');
        const text = el.text || el.label || el.value;
        if (text !== undefined) return `<div class="message-text">${esc(text)}</div>`;
        return `<div class="card-unknown">[phần tử "${esc(el.type || '?')}" chưa được render]</div>`;
      }
    }
  }

  function cardHTML(m) {
    const buttons = Array.isArray(m.card.elements)
      ? '' // nut nam trong elements, da render o tren
      : '';
    return `<div class="task-card">${cardHeaderHTML(m.card)}` +
      cardElementHTML(m.card.elements || m.card.body || [], m.id) + buttons + `</div>`;
  }

  // -------------------------------------------------------------- bubble + list
  function quoteHTML(m, items) {
    if (!m.replyToId) return '';
    const parent = items.find(x => x.id === m.replyToId);
    const who = parent ? senderOf(parent).name : 'Tin nhắn';
    const text = parent ? (parent.text || (parent.card && parent.card.header && parent.card.header.title) || '[card]') : 'Không còn trong trang này';
    return `<button type="button" class="reply-quote" data-jump="${esc(m.replyToId)}">` +
      `<strong>${esc(who)}</strong><span>${esc(String(text).slice(0, 90))}</span></button>`;
  }

  function attachmentsHTML(m) {
    if (!m.attachments || !m.attachments.length) return '';
    return `<div class="message-files">` + m.attachments.map(a => {
      const name = a.name || a.Name || 'Tệp';
      const url = a.url || a.URL || '#';
      const kind = (a.kind || a.Kind || '').toLowerCase();
      if (kind === 'image') {
        return `<a class="file-card" href="${esc(url)}" target="_blank" rel="noreferrer">` +
          `<img src="${esc(url)}" alt="${esc(name)}" loading="lazy" style="max-width:220px;border-radius:9px"></a>`;
      }
      return `<a class="file-card" href="${esc(url)}" target="_blank" rel="noreferrer">` +
        `${icon('file')}<span>${esc(name)}</span></a>`;
    }).join('') + `</div>`;
  }

  function messageHTML(m, place, items) {
    if (m.senderType === 'system' && !m.card) {
      return `<div class="day-label">${esc(m.text)}</div>`;
    }
    const p = senderOf(m);
    const body = m.card ? cardHTML(m)
      : `${quoteHTML(m, items)}${m.text ? `<div class="message-text">${esc(m.text).replace(/\n/g, '<br>')}</div>` : ''}`;

    const replies = m.threadCount || m.replyCount || 0;
    const discussion = place === 'group' && (m.card || replies)
      ? `<div class="task-discussion"><button type="button" class="discussion-button" data-thread="${esc(m.id)}">` +
        `<span class="mini-avatars" aria-hidden="true">${icon('chat')}</span>` +
        `<span>${replies ? replies + ' phản hồi' : 'Mở thread'}</span>${icon('arrow-right')}</button></div>`
      : '';

    const pinned = m.pinned ? `<span class="demo-label" title="Đã ghim">📌</span>` : '';

    return `<article class="message ${isMine(m) ? 'outgoing' : ''}" data-message="${esc(m.id)}">` +
      `<div class="avatar" aria-hidden="true">${esc(p.initial)}</div>` +
      `<div class="message-content">` +
      `<div class="message-head"><span class="sender">${esc(p.name)}</span>` +
      (p.role ? `<span class="live-sender">· ${esc(p.role)}</span>` : '') + `</div>` +
      body + attachmentsHTML(m) +
      `<div class="message-meta">${pinned}<span class="timestamp">${esc(hhmm(m.createdAt))}</span></div>` +
      discussion +
      `<div class="message-actions">` +
      `<button type="button" data-reply="${esc(m.id)}" data-place="${place}" title="Trả lời">${icon('reply')}</button>` +
      `</div></div></article>`;
  }

  function listHTML(items, place) {
    if (!items.length) return `<div class="live-empty">Chưa có tin nhắn nào trong nhóm này.</div>`;
    const out = [];
    let day = '';
    for (const m of items) {
      const key = dayKey(m.createdAt);
      if (key !== day) { out.push(`<div class="day-label">${esc(dayLabel(m.createdAt))}</div>`); day = key; }
      out.push(messageHTML(m, place, items));
    }
    return out.join('');
  }

  // ------------------------------------------------------------------ render
  function renderSidebar() {
    const q = norm($('department-search') ? $('department-search').value : '');
    const list = S.groups.filter(g => norm(g.name).includes(q));
    if ($('group-count')) $('group-count').textContent = S.groups.length;

    $('department-list').innerHTML = list.map(g => {
      const b = S.msgs.get(g.id);
      const last = b && b.items.length ? b.items[b.items.length - 1] : null;
      const preview = last
        ? (last.card ? '📋 ' + ((last.card.header && last.card.header.title) || 'Yêu cầu')
          : (senderOf(last).name + ': ' + (last.text || '[tệp]')))
        : (g.source === 'cms' ? 'Nhóm của CMS' : 'Nhóm nội bộ');
      const badge = g.unreadCount
        ? `<span class="group-unread-count" aria-label="${g.unreadCount} tin chưa xem">${g.unreadCount > 99 ? '99+' : g.unreadCount}</span>` : '';
      return `<button type="button" class="department-row" data-group="${esc(g.id)}" ` +
        `aria-pressed="${g.id === S.activeId}" title="${esc(g.name)}">` +
        `<span class="dept-avatar">${esc(initial(g.name))}</span>` +
        `<span class="dept-copy"><span class="dept-topline">` +
        `<span class="dept-name">${esc(g.name)}</span>` +
        `<span class="dept-time">${esc(last ? hhmm(last.createdAt) : '')}</span>${badge}</span>` +
        `<span class="dept-preview">${esc(String(preview).slice(0, 70))}</span></span></button>`;
    }).join('') || `<div class="live-empty">Không có nhóm nào. Kiểm tra đồng bộ CMS.</div>`;
  }

  function renderHeader() {
    const g = activeGroup();
    if (!g) return;
    $('group-avatar').textContent = initial(g.name);
    $('group-title').textContent = g.name;

    const online = g.members.filter(m => m.presence === 'online').length;
    const typingNames = [...(S.typing.get(g.id) || new Map()).keys()]
      .map(uid => (g.members.find(m => m.userId === uid) || {}).name)
      .filter(Boolean);
    const teams = (g.cms && g.cms.teams || []).map(t => t.code).join(', ');
    $('group-subtitle').textContent = typingNames.length
      ? typingNames.slice(0, 2).join(', ') + ' đang nhập…'
      : `${g.members.length} thành viên · ${online} đang online` + (teams ? ` · ${teams}` : '');

    const req = $('new-request');
    if (req) req.hidden = g.source !== 'cms';
  }

  function renderMessages(keepScroll) {
    const g = activeGroup();
    if (!g) return;
    const box = $('group-messages');
    const b = bucket(g.id);
    const atBottom = box.scrollHeight - box.scrollTop - box.clientHeight < 80;
    const prevHeight = box.scrollHeight, prevTop = box.scrollTop;

    box.innerHTML = (b.pageInfo && b.pageInfo.hasMore
      ? `<button type="button" class="subtle-button" id="load-older" style="margin:8px auto;display:block">Tải tin cũ hơn</button>` : '')
      + listHTML(b.items, 'group');

    if (keepScroll === 'top') box.scrollTop = box.scrollHeight - prevHeight + prevTop;
    else if (atBottom || keepScroll === 'bottom') box.scrollTop = box.scrollHeight;

    const bar = $('unread-group');
    if (bar) {
      bar.hidden = !g.unreadCount;
      bar.textContent = g.unreadCount ? `${g.unreadCount} tin chưa xem` : '';
    }
  }

  function renderThread() {
    const panel = $('thread-panel');
    // .app can lop thread-open de chuyen sang luoi 3 cot (CSS cua demo).
    $('app').classList.toggle('thread-open', !!S.thread);
    if (!S.thread) { panel.hidden = true; return; }
    panel.hidden = false;
    const root = S.thread.root;
    $('thread-title').textContent = root && root.card && root.card.header && root.card.header.title
      ? root.card.header.title : 'Thảo luận';
    $('thread-meta').textContent = `${S.thread.items.length} phản hồi`;
    $('thread-root').innerHTML = root ? messageHTML(root, 'thread', [root]) : '';
    $('thread-messages').innerHTML = listHTML(S.thread.items, 'thread');
    $('thread-messages').scrollTop = $('thread-messages').scrollHeight;
  }

  function renderReplyContext() {
    for (const place of ['group', 'thread']) {
      const el = $(place + '-context');
      if (!el) continue;
      const r = S.replyTo && S.replyTo.place === place ? S.replyTo : null;
      el.hidden = !r;
      el.innerHTML = r
        ? `<div><strong>Trả lời ${esc(r.name)}</strong><span>${esc(String(r.text).slice(0, 80))}</span></div>` +
          `<button type="button" class="icon-button" data-cancel-reply="${place}" title="Bỏ trả lời">${icon('close')}</button>`
        : '';
    }
  }

  function renderMe() {
    const box = $('storage-status');
    if (!box) return;
    const wsOn = S.ws && S.ws.readyState === 1;
    box.innerHTML = S.me
      ? `${esc(S.me.name)} · <span class="live-conn ${wsOn ? 'on' : 'off'}">${wsOn ? 'realtime' : 'mất kết nối'}</span>`
      : 'Chưa đăng nhập';
  }

  function renderRequestBar() {
    const bar = $('request-mode-bar');
    if (!bar) return;
    bar.hidden = !S.requestMode;
    const g = activeGroup();
    const sel = $('request-team');
    if (S.requestMode && g && sel) {
      const teams = (g.cms && g.cms.teams) || [];
      sel.innerHTML = teams.length
        ? teams.map(t => `<option value="${esc(t.teamId ?? t.id ?? '')}">${esc(t.name || t.code)}</option>`).join('')
        : `<option value="">(nhóm chưa gắn tổ nào)</option>`;
      const designer = $('request-designer');
      if (designer) {
        designer.innerHTML = `<option value="">Để CMS chọn người nhận</option>` +
          g.members.filter(m => m.title === 'designer')
            .map(m => `<option value="${esc(m.actorId)}">${esc(m.name)}</option>`).join('');
      }
    }
    const help = $('composer-help');
    if (help) {
      help.textContent = S.requestMode
        ? 'Dòng đầu = tiêu đề yêu cầu, các dòng sau = nội dung brief. Enter để gửi.'
        : 'Enter để gửi · Shift+Enter xuống dòng';
    }
  }

  function renderAll() { renderSidebar(); renderHeader(); renderMessages(); renderMe(); renderRequestBar(); }

  // --------------------------------------------------------------- tai du lieu
  async function openGroup(id, force) {
    S.activeId = id;
    S.thread = null;
    S.replyTo = null;
    const g = activeGroup();
    if (!g) return;
    renderSidebar(); renderHeader(); renderThread(); renderReplyContext(); renderRequestBar();

    const b = bucket(id);
    if (!b.loaded || force) {
      try {
        const el = await Api.messages(groupKey(g), { limit: 40 });
        b.items = (el.messages || []).map(normalize).reverse();  // API tra moi -> cu
        b.pageInfo = el.pageInfo || {};
        b.loaded = true;
      } catch (e) { toast('Không tải được tin nhắn: ' + e.message, 'error'); return; }
    }
    renderMessages('bottom');
    markReadNow();
  }

  async function loadOlder() {
    const g = activeGroup();
    const b = bucket(g.id);
    if (!b.pageInfo || !b.pageInfo.nextCursor) return;
    try {
      const el = await Api.messages(groupKey(g), { limit: 40, cursor: b.pageInfo.nextCursor });
      const older = (el.messages || []).map(normalize).reverse();
      b.items = [...older, ...b.items];
      b.pageInfo = el.pageInfo || {};
      renderMessages('top');
    } catch (e) { toast('Không tải thêm được: ' + e.message, 'error'); }
  }

  let readTimer = null;
  function markReadNow() {
    const g = activeGroup();
    if (!g) return;
    const b = bucket(g.id);
    const last = b.items[b.items.length - 1];
    if (!last) return;
    clearTimeout(readTimer);
    readTimer = setTimeout(async () => {
      try {
        const el = await Api.markRead(groupKey(g), last.id);
        g.unreadCount = (el && el.unreadCount) || 0;
        renderSidebar();
        const bar = $('unread-group');
        if (bar) bar.hidden = true;
      } catch (_) { /* khong chan giao dien vi viec danh dau da xem */ }
    }, 400);
  }

  async function openThread(rootId) {
    try {
      const el = await Api.replies(rootId, 'thread');
      S.thread = {
        rootId,
        root: normalize(el.root),
        items: (el.replies || []).map(normalize).filter(m => m.id !== rootId),
      };
      renderThread();
    } catch (e) { toast('Không mở được thread: ' + e.message, 'error'); }
  }

  // ------------------------------------------------------------------ gui tin
  function upsert(list, m) {
    const i = list.findIndex(x => x.id === m.id ||
      (m.clientMsgId && x.clientMsgId && x.clientMsgId === m.clientMsgId));
    if (i >= 0) { list[i] = m; return false; }
    list.push(m);
    list.sort((a, b) => new Date(a.createdAt) - new Date(b.createdAt));
    return true;
  }

  function receive(m) {
    const gid = m.conversationId;
    const g = S.groups.find(x => x.id === gid);
    if (!g) { bootstrap(); return; }            // nhom moi -> tai lai danh sach

    const b = bucket(gid);
    const fresh = upsert(b.items, m);

    if (m.threadRootId && S.thread && S.thread.rootId === m.threadRootId) {
      upsert(S.thread.items, m);
      renderThread();
    }
    if (S.thread && m.id === S.thread.rootId) { S.thread.root = m; renderThread(); }

    if (gid === S.activeId) {
      renderMessages();
      if (fresh && !isMine(m)) markReadNow();
    } else if (fresh && !isMine(m)) {
      g.unreadCount = (g.unreadCount || 0) + 1;
    }
    renderSidebar();
  }

  async function sendText(place) {
    const g = activeGroup();
    if (!g) return;
    const editor = $(place === 'thread' ? 'thread-input' : 'group-input');
    const text = (editor.innerText || '').replace(/ /g, ' ').trim();
    if (!text) return;

    if (S.requestMode && place === 'group') { await submitRequest(text); return; }

    const clientMsgId = uuid();
    const payload = {
      conversation_id: g.id,
      content: text,
      client_msg_id: clientMsgId,
      thread_id: place === 'thread' && S.thread ? S.thread.rootId : '',
      reply_to_id: S.replyTo && S.replyTo.place === place ? S.replyTo.id : '',
    };

    editor.innerHTML = '';
    S.replyTo = null;
    renderReplyContext();

    // Uu tien WebSocket; dut thi goi REST (cung mot ket qua).
    if (S.ws && S.ws.readyState === 1) {
      S.ws.send(JSON.stringify({ id: clientMsgId, type: 'message.send', payload }));
      return;
    }
    try {
      const el = await Api.send(groupKey(g), {
        text,
        clientMsgId,
        threadRootId: payload.thread_id || undefined,
        replyToId: payload.reply_to_id || undefined,
      });
      if (el && el.message) receive(normalize(el.message));
      toast('Đã gửi qua REST (WebSocket đang mất kết nối)');
    } catch (e) { toast('Gửi thất bại: ' + e.message, 'error'); }
  }

  async function submitRequest(text) {
    const g = activeGroup();
    const [title, ...rest] = text.split('\n');
    const teamId = $('request-team') ? $('request-team').value : '';
    const assignee = $('request-designer') ? $('request-designer').value : '';
    try {
      const el = await Api.submitForm('create_task', {
        kind: 'task',
        title: title.trim(),
        brief: rest.join('\n').trim(),
        teamId,
        useAI: false,
        ...(assignee ? { assigneeActorId: assignee } : {}),
      }, { groupId: groupKey(g) });
      const t = el && el.toast;
      toast(t ? t.text : 'Đã gửi yêu cầu sang CMS', t && t.type === 'error' ? 'error' : '');
      if (!t || t.type !== 'error') {
        $('group-input').innerHTML = '';
        if (el && el.closeForm !== false) { S.requestMode = false; renderRequestBar(); }
      }
    } catch (e) { toast('Không tạo được yêu cầu: ' + e.message, 'error'); }
  }

  async function runAction(messageId, action, values) {
    try {
      const el = await Api.action(messageId, action, values || {});
      const t = el && el.toast;
      if (t) toast(t.text, t.type === 'error' ? 'error' : '');
      else toast('Đã gửi');
      // Card moi se ve qua realtime; neu CMS tra ngay thi ap luon.
      if (el && el.card) {
        for (const b of S.msgs.values()) {
          const m = b.items.find(x => x.id === messageId);
          if (m && (el.cardVersion || 0) >= (m.cardVersion || 0)) {
            m.card = el.card; m.cardVersion = el.cardVersion || m.cardVersion; m.actions = el.actions || m.actions;
          }
        }
        renderMessages(); renderThread();
      }
    } catch (e) {
      toast(e.code === 'action_not_allowed' ? 'Bạn không được phép bấm nút này' : 'Lỗi: ' + e.message, 'error');
    }
  }

  // ----------------------------------------------------------------- realtime
  function connectWS() {
    if (S.ws) { try { S.ws.close(); } catch (_) {} }
    // WebSocket tren CHINH be mat client: cung access token, cung hinh dang
    // field (camelCase) voi REST — khong phai doi ten khoa o FE nua.
    const url = S.base.replace(/^http/, 'ws')
      + '/api/client/v1/chat/ws?token=' + encodeURIComponent(S.token);
    const ws = new WebSocket(url);
    S.ws = ws;

    ws.onopen = () => { S.wsTries = 0; renderMe(); };
    ws.onclose = () => {
      renderMe();
      S.wsTries += 1;
      const wait = Math.min(30000, 1000 * 2 ** Math.min(S.wsTries, 5));
      clearTimeout(S.wsTimer);
      S.wsTimer = setTimeout(() => { if (S.token) connectWS(); }, wait);
    };
    ws.onerror = () => renderMe();
    ws.onmessage = ev => {
      let frame = null;
      try { frame = JSON.parse(ev.data); } catch (_) { return; }
      handleFrame(frame);
    };
  }

  function handleFrame(f) {
    switch (f.type) {
      case 'ready':
        renderMe();
        break;

      case 'ack':
        if (f.payload && f.payload.message) receive(normalize(f.payload.message));
        if (f.payload && f.payload.duplicated) toast('Tin này đã gửi trước đó (client_msg_id trùng)');
        break;

      case 'error':
        toast('Realtime: ' + ((f.payload && f.payload.message) || 'lỗi'), 'error');
        break;

      case 'message':
        if (f.payload) receive(normalize(f.payload));
        break;

      case 'message.deleted': {
        const id = f.payload && (f.payload.message_id || f.payload.id);
        for (const b of S.msgs.values()) {
          const i = b.items.findIndex(x => x.id === id);
          if (i >= 0) b.items.splice(i, 1);
        }
        renderMessages(); renderThread();
        break;
      }

      case 'read':
        // Du lieu da xem cua nguoi khac — hien o day duoi dang nhe nhang.
        renderSidebar();
        break;

      case 'typing': {
        const gid = f.conversation_id;
        const uid = f.payload && (f.payload.user_id || f.payload.userId);
        if (!gid || !uid) break;
        if (!S.typing.has(gid)) S.typing.set(gid, new Map());
        const map = S.typing.get(gid);
        clearTimeout(map.get(uid));
        if (f.payload.typing === false) map.delete(uid);
        else map.set(uid, setTimeout(() => { map.delete(uid); renderHeader(); }, 4000));
        renderHeader();
        break;
      }

      case 'presence': {
        const p = f.payload || {};
        const uid = p.user_id || p.userId;
        const status = p.status || p.presence;
        for (const g of S.groups) {
          const mem = g.members.find(m => m.userId === uid);
          if (mem) mem.presence = status === 'online' ? 'online' : (status === 'off_duty' ? 'off_duty' : 'offline');
        }
        renderHeader();
        break;
      }

      case 'notification':
        S.unreadNotifications += 1;
        if (f.payload) {
          S.notifications.unshift(f.payload);
          toast(f.payload.text || 'Bạn có thông báo mới');
        }
        break;

      case 'reload':
        bootstrap();
        break;

      default:
        break;
    }
  }

  let typingSent = 0;
  function pingTyping() {
    const g = activeGroup();
    if (!g || !S.ws || S.ws.readyState !== 1) return;
    const now = Date.now();
    if (now - typingSent < 2500) return;
    typingSent = now;
    S.ws.send(JSON.stringify({ id: uuid(), type: 'typing', payload: { conversation_id: g.id, typing: true } }));
  }

  // --------------------------------------------------------------- khoi dong
  async function bootstrap() {
    const el = await Api.bootstrap();
    S.me = el.me;
    S.groups = (el.groups || []).map(g => ({ ...g, members: g.members || [] }));
    S.notifications = el.notifications || [];
    S.unreadNotifications = el.unreadNotifications || 0;

    if (!S.activeId || !S.groups.some(g => g.id === S.activeId)) {
      S.activeId = S.groups.length ? S.groups[0].id : null;
    }
    renderAll();
    if (S.activeId) await openGroup(S.activeId, true);
  }

  async function connect(base, token) {
    S.base = base.replace(/\/+$/, '');
    S.token = token.trim();
    await bootstrap();                    // 401 se nem loi -> hien o o dang nhap
    connectWS();
    $('live-auth').hidden = true;
    renderMe();
  }

  // ------------------------------------------------------------------- su kien
  function bind() {
    // --- dang nhap
    $('live-base').value = store.base;
    $('live-token').value = store.token;
    $('live-connect').addEventListener('click', async () => {
      const base = $('live-base').value.trim() || location.origin;
      const token = $('live-token').value.trim();
      const err = $('live-auth-error');
      err.textContent = '';
      if (!token) { err.textContent = 'Cần dán access token.'; return; }
      try {
        await connect(base, token);
        if ($('live-remember').checked) { store.base = base; store.token = token; }
        else { store.token = ''; }
      } catch (e) {
        err.textContent = e.status === 401
          ? 'Token không hợp lệ hoặc đã hết hạn (401). Lấy token mới rồi thử lại.'
          : `Không kết nối được: ${e.message}${e.code ? ' (' + e.code + ')' : ''}`;
      }
    });
    $('live-token').addEventListener('keydown', e => {
      if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) $('live-connect').click();
    });

    // --- sidebar
    $('department-list').addEventListener('click', e => {
      const row = e.target.closest('[data-group]');
      if (row) openGroup(row.dataset.group);
    });
    $('department-search').addEventListener('input', renderSidebar);

    // --- nut dong bo lai nhom tu CMS
    const refresh = $('new-department');
    if (refresh) {
      refresh.hidden = false;
      refresh.title = 'Đồng bộ lại nhóm từ CMS';
      refresh.addEventListener('click', async () => {
        try { await Api.refreshGroups(); await bootstrap(); toast('Đã đồng bộ lại từ CMS'); }
        catch (e) { toast(e.code === 'cms_disabled' ? 'Chat đang chạy không có CMS' : e.message, 'error'); }
      });
    }

    // --- tin nhan: tra loi / mo thread / bam nut card / nhay den tin duoc trich
    document.addEventListener('click', e => {
      const reply = e.target.closest('[data-reply]');
      if (reply) {
        const id = reply.dataset.reply, place = reply.dataset.place || 'group';
        const pool = place === 'thread' && S.thread ? [S.thread.root, ...S.thread.items] : bucket(S.activeId).items;
        const m = pool.find(x => x && x.id === id);
        if (m) {
          S.replyTo = { place, id, name: senderOf(m).name, text: m.text || '[card]' };
          renderReplyContext();
          $(place === 'thread' ? 'thread-input' : 'group-input').focus();
        }
        return;
      }

      const cancel = e.target.closest('[data-cancel-reply]');
      if (cancel) { S.replyTo = null; renderReplyContext(); return; }

      const thread = e.target.closest('[data-thread]');
      if (thread) { openThread(thread.dataset.thread); return; }

      const jump = e.target.closest('[data-jump]');
      if (jump) {
        const node = document.querySelector(`[data-message="${jump.dataset.jump}"]`);
        if (node) {
          node.scrollIntoView({ behavior: 'smooth', block: 'center' });
          node.style.transition = 'background .4s';
          node.style.background = '#fff6d6';
          setTimeout(() => { node.style.background = ''; }, 1200);
        } else toast('Tin được trích dẫn không còn trong trang này');
        return;
      }

      const action = e.target.closest('.card-action');
      if (action) {
        const confirmText = action.dataset.confirm;
        if (confirmText && !window.confirm(confirmText)) return;
        let values = {};
        if (action.dataset.form) {
          // Card khai bao form -> hoi tung truong bang prompt (du cho ban dev).
          try {
            const fields = JSON.parse(action.dataset.form);
            for (const f of (Array.isArray(fields) ? fields : fields.fields || [])) {
              const name = f.name || f.id;
              if (!name) continue;
              const answer = window.prompt(f.label || name, f.value || '');
              if (answer === null) return;
              values[name] = answer;
            }
          } catch (_) { /* form khong doc duoc -> gui khong kem values */ }
        }
        runAction(action.dataset.message, action.dataset.action, values);
        return;
      }

      if (e.target.closest('#load-older')) loadOlder();
      if (e.target.closest('#close-thread')) { S.thread = null; renderThread(); }
    });

    // --- soan tin: Enter gui, Shift+Enter xuong dong
    for (const place of ['group', 'thread']) {
      const editor = $(place + '-input');
      const form = $(place + '-form');
      if (editor) {
        editor.addEventListener('keydown', e => {
          if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendText(place); }
        });
        editor.addEventListener('input', pingTyping);
      }
      if (form) form.addEventListener('submit', e => { e.preventDefault(); sendText(place); });
    }

    // --- cuon len dau de tai tin cu
    $('group-messages').addEventListener('scroll', () => {
      if ($('group-messages').scrollTop < 40) {
        const b = bucket(S.activeId);
        if (b.pageInfo && b.pageInfo.nextCursor && !b.loading) {
          b.loading = true;
          loadOlder().finally(() => { b.loading = false; });
        }
      }
    });

    // --- tao yeu cau
    const newReq = $('new-request');
    if (newReq) newReq.addEventListener('click', () => {
      S.requestMode = true; renderRequestBar(); $('group-input').focus();
    });
    const cancelReq = $('cancel-request');
    if (cancelReq) cancelReq.addEventListener('click', () => { S.requestMode = false; renderRequestBar(); });

    // --- thong tin nhom + thong bao
    const info = $('open-info');
    if (info) info.addEventListener('click', () => {
      const g = activeGroup();
      const dlg = $('info-dialog');
      if (!g || !dlg) return;
      const title = $('info-dialog-title');
      if (title) title.textContent = g.name;
      const members = g.members.map(m =>
        `<li>${esc(m.name || m.userId)} — ${esc(m.title || m.role)}` +
        `${m.presence === 'online' ? ' · online' : m.presence === 'off_duty' ? ' · OFF' : ''}` +
        `${(m.sources || []).length ? ' · ' + esc((m.sources || []).join(', ')) : ''}</li>`).join('');
      const notis = S.notifications.slice(0, 10).map(n =>
        `<li>${esc(n.text || '')}</li>`).join('') || '<li>Chưa có thông báo</li>';
      const body = dlg.querySelector('.info-body') || dlg;
      const extra = document.createElement('div');
      extra.innerHTML = `<p><strong>chatId:</strong> <code>${esc(g.chatId || '(nội bộ)')}</code> · ` +
        `<strong>nguồn:</strong> ${esc(g.source)}</p>` +
        `<p><strong>Thành viên (${g.members.length})</strong></p><ul>${members}</ul>` +
        `<p><strong>Thông báo (${S.unreadNotifications} chưa đọc)</strong></p><ul>${notis}</ul>`;
      const old = dlg.querySelector('#live-info-extra');
      if (old) old.remove();
      extra.id = 'live-info-extra';
      body.appendChild(extra);
      if (typeof dlg.showModal === 'function') dlg.showModal(); else dlg.hidden = false;
    });
    const infoClose = $('info-close');
    if (infoClose) infoClose.addEventListener('click', () => {
      const dlg = $('info-dialog');
      if (dlg && typeof dlg.close === 'function') dlg.close(); else if (dlg) dlg.hidden = true;
    });

    // --- menu noi cua demo khong dung o ban live
    for (const id of ['sidebar-menu-toggle', 'group-menu-toggle', 'toggle-search']) {
      const el = $(id);
      if (el) el.hidden = true;
    }

    // --- mo lai WS khi tab duoc xem lai
    document.addEventListener('visibilitychange', () => {
      if (!document.hidden && S.token && (!S.ws || S.ws.readyState > 1)) connectWS();
    });
  }

  // ------------------------------------------------------------------- chay
  document.addEventListener('DOMContentLoaded', async () => {
    bind();
    if (store.token) {
      try { await connect(store.base, store.token); }
      catch (e) {
        $('live-auth').hidden = false;
        $('live-auth-error').textContent = e.status === 401
          ? 'Token đã lưu không còn hiệu lực. Dán token mới.' : e.message;
      }
    } else {
      $('live-auth').hidden = false;
    }
  });
})();
