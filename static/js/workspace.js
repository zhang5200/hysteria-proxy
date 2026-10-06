let membership = null;
let redemptionCards = [];
let selectedPlan = "monthly";
let cardFilter = "all";
let activeWorkspaceTab = "overview";
let toastTimeout;
const planNames = { monthly: "月卡", yearly: "年卡" };
const escapeHTML = (value) =>
  String(value ?? "").replace(
    /[&<>"']/g,
    (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        c
      ],
  );
const money = (cents) =>
  (cents / 100).toLocaleString("zh-CN", { maximumFractionDigits: 2 });
const dateText = (timestamp) =>
  timestamp
    ? new Date(timestamp * 1000).toLocaleString("zh-CN", {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
      })
    : "—";
function toast(message) {
  const el = document.getElementById("toast");
  el.textContent = message;
  el.classList.add("visible");
  clearTimeout(toastTimeout);
  toastTimeout = setTimeout(() => el.classList.remove("visible"), 3200);
}
async function workspaceRequest(url, options) {
  const response = await apiFetch(url, options);
  if (!response.ok) {
    if (response.status === 401) {
      sessionStorage.clear();
      document.getElementById("login-overlay").style.cssText = "";
      document.getElementById("mainApp").style.opacity = "0";
      document.getElementById("mainApp").inert = true;
      document
        .querySelectorAll(".modal.active")
        .forEach((modal) => modal.classList.remove("active"));
    }
    throw new Error(
      response.status === 401
        ? "登录已过期，请重新登录"
        : (await response.text()).trim() || "请求失败，请重试",
    );
  }
  return response.json();
}
async function loadWorkspace() {
  try {
    membership = await workspaceRequest("/api/membership");
    if (membership.role === "admin")
      redemptionCards = await workspaceRequest("/api/cards");
    renderWorkspace();
  } catch (error) {
    toast(error.message);
    document.getElementById("overviewStats").innerHTML =
      '<div class="stat-card empty-state" style="grid-column:1/-1">加载失败，请检查连接后 <button class="btn btn-light" onclick="loadWorkspace()">重试</button></div>';
  }
}
function renderWorkspace() {
  const admin = currentUserRole === "admin";
  const used = redemptionCards.filter((c) => c.redeemed_at);
  const expiry = membership.expires_at;
  const unlimited = membership.user_id && !expiry.Valid;
  const days = unlimited
    ? "不限时"
    : Math.max(0, Math.ceil((expiry.Int64 - Date.now() / 1000) / 86400));
  const stat = (label, value, note, symbol) =>
    `<div class="stat-card"><div class="stat-label">${label}<span class="stat-symbol">${symbol}</span></div><div class="stat-value">${value}</div><div class="stat-note">${note}</div></div>`;
  document.getElementById("overviewStats").innerHTML = admin
    ? stat(
        "累计生成",
        `${redemptionCards.length} <small>张</small>`,
        "所有月卡与年卡",
        "▤",
      ) +
      stat(
        "待兑换卡密",
        `${redemptionCards.length - used.length} <small>张</small>`,
        "等待开启下一段连接",
        "◇",
      ) +
      stat(
        "已成功兑换",
        `${used.length} <small>张</small>`,
        "使用时长已到账",
        "✓",
      ) +
      stat(
        "已兑换面值",
        `<small>¥</small> ${money(used.reduce((sum, c) => sum + c.price_cents, 0))}`,
        "按卡密售价统计，非收款金额",
        "↗",
      )
    : stat(
        "剩余时长",
        `${days}${unlimited ? "" : " <small>天</small>"}`,
        "兑换卡密即可延长有效期",
        "◷",
      ) +
      stat(
        "订阅状态",
        `<small>${!membership.user_id ? "待关联" : !membership.enabled ? "已停用" : unlimited || days > 0 ? "使用中" : "待兑换"}</small>`,
        "你的专属连接",
        "◉",
      ) +
      stat(
        "累计兑换",
        `${membership.history.length} <small>次</small>`,
        "每一段时光，都有记录",
        "✓",
      ) +
      stat(
        "到期日期",
        `<small>${unlimited ? "不限时" : expiry.Int64 ? dateText(expiry.Int64).split(" ")[0] : "尚未激活"}</small>`,
        "有效期内可继续叠加",
        "▤",
      );
  document.getElementById("planCards").innerHTML = ["monthly", "yearly"]
    .map(
      (kind) =>
        `<article class="plan-card ${kind}"><div class="plan-top"><div class="plan-name"><div class="plan-icon">${kind === "monthly" ? "◷" : "✳"}</div><div><h3>${planNames[kind]}</h3><small>${kind === "monthly" ? "MONTHLY PASS" : "ANNUAL PASS"}</small></div></div><span class="plan-tag">${kind === "monthly" ? "轻松开始" : "长久陪伴"}</span></div><div class="plan-price"><sup>¥</sup>${money(membership.plans[kind])}<small>/ ${kind === "monthly" ? "30" : "365"} 天</small></div><div class="plan-description">${kind === "monthly" ? "给自己一个月，探索更多可能。" : "把一整年的自由，提前安排好。"}</div><div class="plan-features"><span>${kind === "monthly" ? "30" : "365"} 天使用时长</span><span>兑换即时生效</span><span>支持时长叠加</span></div><button class="btn btn-light" onclick="${admin ? `openGenerate('${kind}')` : "switchTab('redeem')"}">${admin ? "设置价格并生成卡密" : "已有卡密，前往兑换"} <span>↗</span></button></article>`,
    )
    .join("");
  document.getElementById("membershipInfo").innerHTML =
    `<h3>我的订阅 <span class="mini-label">MEMBERSHIP</span></h3><span class="badge badge-success">${!membership.user_id ? "未关联 VPN 用户" : !membership.enabled ? "账户已停用" : unlimited ? "不限时订阅" : days > 0 ? "订阅有效" : "等待开启"}</span><div class="membership-days">${days}${unlimited ? "" : " <small>天剩余</small>"}</div><p>${unlimited ? "当前账户没有时长限制" : `有效期至：${dateText(expiry.Int64)}`}</p><p>兑换后，使用时长将自动累计。</p>${membership.user_id ? `<button class="btn btn-light" onclick="showSubscriptionModal(${membership.user_id})">获取订阅链接 ↗</button>` : "<p>管理账号如需使用 VPN，请先关联同名 VPN 用户。</p>"}`;
  document.getElementById("redeemHistory").innerHTML = membership.history.length
    ? membership.history
        .map(
          (c) =>
            `<tr><td><code>${escapeHTML(c.code)}</code></td><td>${planNames[c.kind]}</td><td>${dateText(c.redeemed_at)}</td><td>${dateText(c.expires_at)}</td></tr>`,
        )
        .join("")
    : '<tr><td colspan="4" class="empty-state">还没有兑换记录<br>兑换第一张卡密，开启你的专属连接。</td></tr>';
  renderCards();
}
function renderCards() {
  const search = document
    .getElementById("cardSearch")
    .value.trim()
    .toLowerCase();
  const cards = redemptionCards.filter(
    (c) =>
      (cardFilter === "all" ||
        (cardFilter === "used" ? c.redeemed_at : !c.redeemed_at)) &&
      `${c.code} ${c.redeemed_by}`.toLowerCase().includes(search),
  );
  document.getElementById("cardRows").innerHTML = cards.length
    ? cards
        .map(
          (c) =>
            `<tr><td><code>${escapeHTML(c.code)}</code></td><td>${planNames[c.kind]} <small>${c.kind === "monthly" ? 30 : 365} 天使用时长</small></td><td>¥ ${money(c.price_cents)}</td><td><span class="badge ${c.redeemed_at ? "badge-primary" : "badge-success"}">${c.redeemed_at ? "已兑换" : "未兑换"}</span></td><td>${escapeHTML(c.redeemed_by || "等待兑换")}<small>${dateText(c.redeemed_at)}</small></td><td>${dateText(c.created_at)}</td><td><button class="btn btn-light" onclick="copyCard(${c.id})">复制</button></td></tr>`,
        )
        .join("")
    : `<tr><td colspan="7" class="empty-state">${redemptionCards.length ? "没有匹配的卡密，试试其他关键词或状态。" : "还没有卡密。点击「生成卡密」，创建你的第一张月卡或年卡。"}</td></tr>`;
  document.getElementById("cardCount").textContent =
    `共 ${cards.length} 张卡密 · 每张仅可兑换一次`;
}
function filterCards(filter, button) {
  cardFilter = filter;
  document
    .querySelectorAll(".filter-tabs button")
    .forEach((b) => b.classList.toggle("selected", b === button));
  renderCards();
}
function selectPlan(kind) {
  selectedPlan = kind;
  document
    .getElementById("chooseMonthly")
    .classList.toggle("selected", kind === "monthly");
  document
    .getElementById("chooseYearly")
    .classList.toggle("selected", kind === "yearly");
  document.getElementById("cardPrice").value =
    (membership?.plans[kind] ?? 0) / 100;
}
async function openGenerate(kind = "monthly") {
  if (currentUserRole !== "admin") return;
  if (!membership) await loadWorkspace();
  if (!membership) return;
  selectPlan(kind);
  document.getElementById("cardQuantity").value = 1;
  document.getElementById("generateError").textContent = "";
  document.getElementById("generatedResult").hidden = true;
  document.getElementById("generateModal").classList.add("active");
  document.getElementById("cardPrice").focus();
}
function closeGenerate() {
  if (!document.getElementById("generateButton").disabled)
    document.getElementById("generateModal").classList.remove("active");
}
async function generateCards(event) {
  event.preventDefault();
  const button = document.getElementById("generateButton");
  if (button.disabled) return;
  button.disabled = true;
  button.textContent = "正在生成…";
  document.getElementById("generateError").textContent = "";
  document.getElementById("generatedResult").hidden = true;
  try {
    const result = await workspaceRequest("/api/cards", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        kind: selectedPlan,
        price_cents: Math.round(
          Number(document.getElementById("cardPrice").value) * 100,
        ),
        quantity: Number(document.getElementById("cardQuantity").value),
      }),
    });
    document.getElementById("generatedCodes").value = result
      .map((c) => c.code)
      .join("\n");
    document.getElementById("generatedResult").hidden = false;
    toast(`已生成 ${result.length} 张${planNames[selectedPlan]}`);
    await loadWorkspace();
  } catch (error) {
    document.getElementById("generateError").textContent = error.message;
  } finally {
    button.disabled = false;
    button.textContent = "确认生成卡密 →";
  }
}
async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text);
    toast("已复制到剪贴板");
  } catch {
    const area = document.createElement("textarea");
    area.value = text;
    area.style.cssText = "position:fixed;left:-9999px";
    document.body.appendChild(area);
    area.select();
    const success = document.execCommand("copy");
    area.remove();
    toast(success ? "已复制到剪贴板" : "复制失败，请选中文字手动复制");
  }
}
function copyCard(id) {
  const card = redemptionCards.find((c) => c.id === id);
  if (card) copyText(card.code);
}
function copyGenerated() {
  copyText(document.getElementById("generatedCodes").value);
}
async function redeemCard(event) {
  event.preventDefault();
  const button = document.getElementById("redeemButton");
  if (button.disabled) return;
  const result = document.getElementById("redeemResult");
  button.disabled = true;
  button.textContent = "正在兑换…";
  result.textContent = "";
  try {
    const data = await workspaceRequest("/api/redeem", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        code: document.getElementById("redeemCode").value.trim(),
      }),
    });
    result.style.color = "var(--primary)";
    result.textContent = `✓ 兑换成功，已增加 ${data.days} 天！有效期至 ${dateText(data.expires_at)}。`;
    document.getElementById("redeemCode").value = "";
    await loadWorkspace();
  } catch (error) {
    result.style.color = "var(--danger)";
    result.textContent = error.message;
  } finally {
    button.disabled = false;
    button.innerHTML = "兑换使用时长 <span>→</span>";
  }
}
// Extend the original management screens without changing their subscription actions.
const originalRoleUI = updateUIForRole;
updateUIForRole = function (role) {
  originalRoleUI(role);
  const admin = role === "admin";
  document.querySelector('[data-tab="cards"]').style.display = admin
    ? "flex"
    : "none";
  document.querySelector(".nav-admin").style.display = admin ? "" : "none";
  document.querySelector("#usersTab h3").textContent = admin
    ? "用户列表"
    : "我的订阅";
  const usersMenu = document.querySelector('[data-tab="users"]');
  if (usersMenu.lastChild.nodeType === Node.TEXT_NODE)
    usersMenu.lastChild.textContent = admin ? "用户管理" : "我的订阅";
  document.querySelector(".avatar").textContent = (
    sessionStorage.getItem("username") || "U"
  )
    .slice(0, 1)
    .toUpperCase();
  document.getElementById("welcomeTitle").textContent = admin
    ? "每一次连接，都值得期待。"
    : "你好，世界。欢迎自由连接。";
  document.getElementById("welcomeDescription").textContent = admin
    ? "管理你的套餐与卡密，让每一份连接井然有序。"
    : "在这里，查看订阅、兑换时长，开启你的下一段旅程。";
  document.getElementById("plansDescription").textContent = admin
    ? "两种时长，一样的自在体验。定价由你掌握。"
    : "选择适合自己的时长，联系服务商获取卡密后即可兑换。";
  const action = document.getElementById("overviewAction");
  action.textContent = admin ? "＋ 生成卡密" : "兑换卡密 ↗";
  action.onclick = admin ? () => openGenerate() : () => switchTab("redeem");
};
const originalSwitchTab = switchTab;
switchTab = function (tab) {
  if (
    currentUserRole !== "admin" &&
    ["cards", "nodes", "account"].includes(tab)
  )
    tab = "overview";
  activeWorkspaceTab = tab;
  document.querySelector(".page-title").textContent =
    {
      overview: "总览",
      cards: "卡密管理",
      redeem: "兑换中心",
      users: currentUserRole === "admin" ? "用户管理" : "我的订阅",
      nodes: "节点管理",
      account: "账号管理",
    }[tab] || "总览";
  if (["overview", "cards", "redeem"].includes(tab)) {
    document
      .querySelectorAll(".tab-content")
      .forEach((el) => el.classList.toggle("active", el.id === `${tab}Tab`));
    document
      .querySelectorAll(".menu-item")
      .forEach((el) => el.classList.toggle("active", el.dataset.tab === tab));
    loadWorkspace();
  } else originalSwitchTab(tab);
  document.querySelector(".content-scroll-area").scrollTop = 0;
};
// Escape and close accessible dialogs with the keyboard.
document.querySelectorAll(".modal").forEach((modal) => {
  modal.setAttribute("role", "dialog");
  modal.setAttribute("aria-modal", "true");
  const title = modal.querySelector("h3");
  if (title) {
    if (!title.id) title.id = `${modal.id}Title`;
    modal.setAttribute("aria-labelledby", title.id);
  }
  const close = modal.querySelector("span.close-btn");
  if (close) {
    close.setAttribute("role", "button");
    close.tabIndex = 0;
    close.setAttribute("aria-label", "关闭");
    close.addEventListener("keydown", (event) => {
      if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        close.click();
      }
    });
  }
  modal.addEventListener("click", (event) => {
    if (event.target === modal && !modal.querySelector("button:disabled"))
      modal.classList.remove("active");
  });
});
document.addEventListener("keydown", (event) => {
  const modal = [...document.querySelectorAll(".modal.active")].at(-1);
  if (!modal) return;
  if (event.key === "Escape" && !modal.querySelector("button:disabled"))
    modal.classList.remove("active");
  if (event.key === "Tab") {
    const focusable = [
      ...modal.querySelectorAll(
        'button:not(:disabled),input,select,textarea,[tabindex="0"]',
      ),
    ].filter((el) => el.getClientRects().length);
    const first = focusable[0],
      last = focusable.at(-1);
    if (
      event.shiftKey &&
      (document.activeElement === first ||
        !modal.contains(document.activeElement))
    ) {
      event.preventDefault();
      last?.focus();
    } else if (
      !event.shiftKey &&
      (document.activeElement === last ||
        !modal.contains(document.activeElement))
    ) {
      event.preventDefault();
      first?.focus();
    }
  }
});
