// Config & Constants
const USER_API = "/api/users";
const NODE_API = "/api/nodes";
const ADMIN_USER_API = "/api/admin-users";
const TRAFFIC_BY_NODE_API = "/api/traffic/by-node";
const LOGIN_API = "/api/login";

let users = [];
let nodes = [];
let adminUsers = [];
let trafficByNode = {};
let editingUserId = null; // Track if we're editing a user
let editingNodeId = null; // Track if we're editing a node
let editingAdminUserId = null; // Track if we're editing an admin user
let currentUserRole = null; // Current user's role

// --- Auth Logic ---
async function checkLogin() {
  const userInput = document.getElementById("loginUsername").value;
  const passInput = document.getElementById("loginPassword").value;
  const error = document.getElementById("loginError");

  try {
    const res = await fetch(LOGIN_API, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        username: userInput,
        password: passInput
      })
    });

    const data = await res.json();

    if (data.success) {
      // Success
      document.getElementById("login-overlay").style.opacity = "0";
      setTimeout(() => {
        document.getElementById("login-overlay").style.display = "none";
        document.getElementById("mainApp").style.opacity = "1";
      }, 300);
      sessionStorage.setItem("isLoggedIn", "true");
      sessionStorage.setItem("userRole", data.role);
      sessionStorage.setItem("username", userInput);
      currentUserRole = data.role;
      
      // Update UI based on role
      updateUIForRole(data.role);
      
      // Load initial data
      if (data.role === "admin") {
        refreshUsers();
      } else {
        switchTab('subscription');
      }
    } else {
      // Fail
      error.textContent = data.message || "账号或密码错误，请重试";
      error.style.display = "block";
      document.getElementById("loginUsername").classList.add("error");
      document.getElementById("loginPassword").classList.add("error");
    }
  } catch (err) {
    console.error("Login error:", err);
    error.textContent = "登录失败，请稍后重试";
    error.style.display = "block";
  }
}

function logout() {
  sessionStorage.removeItem("isLoggedIn");
  sessionStorage.removeItem("userRole");
  sessionStorage.removeItem("username");
  location.reload();
}

// Update UI based on user role
function updateUIForRole(role) {
  const menuItems = document.querySelectorAll(".menu-item");
  const userInfo = document.querySelector(".user-info span");
  const username = sessionStorage.getItem("username") || "User";
  
  if (userInfo) {
    userInfo.textContent = username;
  }

  if (role === "user") {
    // Hide user management, node management and account management for regular users
    menuItems[0].style.display = "none"; // 用户管理
    menuItems[1].style.display = "none"; // 节点管理
    menuItems[2].style.display = "flex"; // VPN订阅
    menuItems[3].style.display = "none"; // 账号管理
  } else {
    // Show all menus for admin
    menuItems.forEach(item => item.style.display = "flex");
  }
}

// Check auth on load
window.addEventListener("load", () => {
  if (sessionStorage.getItem("isLoggedIn") === "true") {
    const role = sessionStorage.getItem("userRole") || "user";
    currentUserRole = role;
    document.getElementById("login-overlay").style.display = "none";
    document.getElementById("mainApp").style.opacity = "1";
    updateUIForRole(role);
    
    if (role === "admin") {
      refreshUsers();
    } else {
      switchTab('subscription');
    }
  }
});

// --- Tab Logic ---
function switchTab(tab) {
  // Remove active class from all menu items and tabs
  document
    .querySelectorAll(".menu-item")
    .forEach((t) => t.classList.remove("active"));
  document
    .querySelectorAll(".tab-content")
    .forEach((c) => c.classList.remove("active"));

  // Find and activate the correct menu item by checking onclick attribute
  const menuItems = document.querySelectorAll(".menu-item");
  menuItems.forEach((item) => {
    const onclick = item.getAttribute("onclick");
    if (onclick && onclick.includes(`'${tab}'`)) {
      item.classList.add("active");
    }
  });

  // Activate the corresponding tab content
  if (tab === "users") {
    document.getElementById("usersTab").classList.add("active");
    loadUsers();
  } else if (tab === "nodes") {
    document.getElementById("nodesTab").classList.add("active");
    loadNodes();
  } else if (tab === "subscription") {
    document.getElementById("subscriptionTab").classList.add("active");
    loadSubscriptions();
  } else if (tab === "account") {
    document.getElementById("accountTab").classList.add("active");
    loadAdminUsers();
  }
}

// --- Data Logic ---
async function loadUsers() {
  try {
    const res = await fetch(USER_API);
    users = await res.json();
    renderUsers();
  } catch (err) {
    console.error("加载用户失败", err);
  }
}

async function loadTrafficByNode() {
  try {
    const res = await fetch(TRAFFIC_BY_NODE_API);
    const data = await res.json();
    trafficByNode = {};
    data.forEach((entry) => {
      if (!trafficByNode[entry.username]) trafficByNode[entry.username] = {};
      if (!trafficByNode[entry.username][entry.node_name])
        trafficByNode[entry.username][entry.node_name] = { tx: 0, rx: 0 };
      trafficByNode[entry.username][entry.node_name].tx += entry.tx;
      trafficByNode[entry.username][entry.node_name].rx += entry.rx;
    });
  } catch (err) {
    console.error("traffic load error", err);
  }
}

function formatBytes(bytes) {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
}

function toggleDetail(username) {
  const row = document.getElementById(`detail-${username}`);
  const icon = document.getElementById(`icon-${username}`);
  if (row.classList.contains("show")) {
    row.classList.remove("show");
    icon.style.transform = "rotate(0deg)";
    icon.style.color = "var(--text-secondary)";
  } else {
    row.classList.add("show");
    icon.style.transform = "rotate(90deg)";
    icon.style.color = "var(--primary)";
  }
}

function toggleNodeTraffic(username) {
  const container = document.getElementById(`node-traffic-${username}`);
  const icon = document.getElementById(`toggle-icon-${username}`);
  
  if (container.style.maxHeight && container.style.maxHeight !== "0px") {
    // Collapse
    container.style.maxHeight = "0px";
    icon.style.transform = "rotate(0deg)";
    icon.style.color = "var(--text-secondary)";
  } else {
    // Expand
    container.style.maxHeight = container.scrollHeight + "px";
    icon.style.transform = "rotate(90deg)";
    icon.style.color = "var(--primary)";
  }
}

function renderUsers() {
  const container = document.getElementById("userTable");
  container.innerHTML = "";
  users.forEach((user) => {
    const card = document.createElement("div");
    card.className = "data-card";

    // Traffic Details check
    let hasTraffic =
      trafficByNode[user.username] &&
      Object.keys(trafficByNode[user.username]).length > 0;
    let trafficHtml = "";

    if (hasTraffic) {
      const nodeCount = Object.keys(trafficByNode[user.username]).length;
      trafficHtml = `
        <div style="margin-top: 10px; border-top: 1px dashed var(--border); padding-top: 10px;">
          <button class="btn" style="width: 100%; padding: 8px 12px; font-size: 12px; justify-content: space-between; background: transparent; border: 1px solid var(--border); border-radius: 6px; color: var(--text-secondary)" 
                  onclick="toggleNodeTraffic('${user.username}')">
            <span style="display: flex; align-items: center; gap: 6px;">
              <svg id="toggle-icon-${user.username}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" 
                   style="width: 14px; height: 14px; transition: transform 0.2s; color: var(--text-secondary);">
                <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
              </svg>
              <span>各节点流量统计</
            </span>
            <span class="badge badge-primary" style="font-size: 11px;">${nodeCount} 个节点</span>
          </button>
          <div id="node-traffic-${user.username}" style="max-height: 0; overflow: hidden; transition: max-height 0.3s ease;">
            <div style="background: #f9fafb; border-radius: 6px; padding: 12px; margin-top: 8px;">
      `;
      for (const [node, t] of Object.entries(trafficByNode[user.username])) {
        trafficHtml += `
          <div style="display: flex; flex-direction: row; justify-content: space-between; align-items: center; gap: 12px; padding: 8px 0;">
            <span style="font-weight: 500; font-size: 12px; color: var(--text); white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">${node}</span>
            <span style="font-family: monospace; font-size: 11px; color: var(--text-secondary); white-space: nowrap; flex-shrink: 0;">
              <span style="color: var(--success)">↑ ${formatBytes(t.tx)}</span> 
              <span style="color: var(--primary); margin-left: 8px;">↓ ${formatBytes(t.rx)}</span>
            </span>
          </div>
        `;
      }
      trafficHtml += `
            </div>
          </div>
        </div>
      `;
    }

    // Calculate traffic usage
    const totalTraffic = (user.tx || 0) + (user.rx || 0);
    const trafficLimit = user.traffic_limit || 0;
    let trafficLimitHtml = "";

    if (trafficLimit > 0) {
      const limitGB = (trafficLimit / 1024 ** 3).toFixed(2);
      const usedGB = (totalTraffic / 1024 ** 3).toFixed(2);
      const percentage = Math.min(
        100,
        (totalTraffic / trafficLimit) * 100
      ).toFixed(1);
      const isExceeded = totalTraffic >= trafficLimit;

      trafficLimitHtml = `
                <div style="margin-top: 10px; font-size: 13px;">
                    <div style="display: flex; justify-content: space-between; margin-bottom: 4px;">
                        <span style="color: ${
                          isExceeded ? "var(--danger)" : "var(--text-secondary)"
                        }">
                            流量: ${usedGB} GB / ${limitGB} GB
                        </span>
                        <span style="font-weight: 600; color: ${
                          isExceeded ? "var(--danger)" : "var(--primary)"
                        }">
                            ${percentage}%
                        </span>
                    </div>
                    <div style="background: #e5e7eb; border-radius: 4px; overflow: hidden; height: 6px;">
                        <div style="background: ${
                          isExceeded ? "var(--danger)" : "var(--primary)"
                        }; 
                                    width: ${percentage}%; height: 100%; transition: width 0.3s;"></div>
                    </div>
                    ${
                      isExceeded
                        ? '<div style="color: var(--danger); margin-top: 4px; font-weight: 600; font-size: 12px;">⚠️ 已超限</div>'
                        : ""
                    }
                </div>
            `;
    } else {
      trafficLimitHtml =
        '<div style="margin-top: 10px; color: var(--text-secondary); font-size: 13px;">流量: 无限制</div>';
    }

    card.innerHTML = `
            <div class="card-header">
                <div class="card-title">
                    <div style="width:32px; height:32px; background:#eff6ff; color:var(--primary); border-radius:50%; display:flex; align-items:center; justify-content:center; font-weight:800; font-size:14px;">
                        ${user.username.charAt(0).toUpperCase()}
                    </div>
                    ${user.username}
                </div>
                <span class="badge ${
                  user.enabled ? "badge-success" : "badge-danger"
                }">${user.enabled ? "启用" : "禁用"}</span>
            </div>
            
            <div class="card-row">
                <span>连接密码:</span>
                <span style="font-family: monospace; background: #f3f4f6; padding: 2px 6px; border-radius: 4px;">${
                  user.password
                }</span>
            </div>
            
            <div class="card-row">
                <span>总流量:</span>
                <span style="font-family: monospace;">
                     <span style="color: var(--success)">↑ ${formatBytes(
                       user.tx || 0
                     )}</span> / 
                     <span style="color: var(--primary)">↓ ${formatBytes(
                       user.rx || 0
                     )}</span>
                </span>
            </div>
            
            ${trafficLimitHtml}
            
            ${trafficHtml}

            <div class="card-actions">
                <button class="btn btn-primary" style="padding: 6px 12px; font-size: 12px;" onclick="showEditUserModal(${
                  user.id
                })">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 14px; height: 14px;">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                    </svg>
                    编辑
                </button>
                <button class="btn ${
                  user.enabled ? "btn-danger" : "btn-success"
                }" style="padding: 6px 12px; font-size: 12px;" onclick="toggleUser(${
      user.id
    }, ${!user.enabled})">
                    ${user.enabled ? "禁用" : "启用"}
                </button>
                ${
                  trafficLimit > 0
                    ? `
                    <button class="btn btn-success" style="padding: 6px 12px; font-size: 12px;" onclick="resetUserTraffic(${user.id})">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 14px; height: 14px;">
                            <path stroke-linecap="round" stroke-linejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                        </svg>
                        重置流量
                    </button>
                `
                    : ""
                }
                <button class="btn btn-danger" style="padding: 6px 12px; font-size: 12px;" onclick="deleteUser(${
                  user.id
                })">删除</button>
            </div>
        `;
    container.appendChild(card);
  });
}

// --- Actions ---
async function saveUser() {
  const u = document.getElementById("usernameInput").value;
  const p = document.getElementById("passwordInput").value;
  const trafficLimitGB = parseInt(
    document.getElementById("trafficLimitInput").value || 0
  );
  const autoDisable = document.getElementById("autoDisableInput").checked;

  if (!u || !p) return alert("请填写完整");

  // Convert GB to bytes
  const trafficLimitBytes = trafficLimitGB * 1024 ** 3;

  try {
    if (editingUserId) {
      // Update existing user - get current user to preserve enabled status
      const currentUser = users.find((user) => user.id === editingUserId);
      await fetch(`${USER_API}/${editingUserId}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          username: u,
          password: p,
          enabled: currentUser ? currentUser.enabled : true,
          traffic_limit: trafficLimitBytes,
          auto_disable_on_limit: autoDisable,
        }),
      });
    } else {
      // Create new user
      await fetch(USER_API, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          username: u,
          password: p,
          traffic_limit: trafficLimitBytes,
          auto_disable_on_limit: autoDisable,
        }),
      });
    }
    closeUserModal();
    loadUsers();
  } catch (e) {
    alert("操作失败");
  }
}

async function deleteUser(id) {
  if (!confirm("确认删除该用户吗？")) return;
  await fetch(`${USER_API}/${id}`, { method: "DELETE" });
  loadUsers();
}

async function toggleUser(id, enabled) {
  const user = users.find((u) => u.id === id);
  if (!user) return;
  await fetch(`${USER_API}/${id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      enabled,
      password: user.password,
      traffic_limit: user.traffic_limit || 0,
      auto_disable_on_limit: user.auto_disable_on_limit !== false,
    }),
  });
  loadUsers();
}

async function resetUserTraffic(id) {
  if (
    !confirm(
      "确定要重置该用户的流量统计吗？此操作将清空流量记录并重新启用用户。"
    )
  )
    return;

  try {
    const res = await fetch(`${USER_API}/${id}/reset-traffic`, {
      method: "POST",
    });
    if (res.ok) {
      alert("流量已重置");
      loadUsers();
    } else {
      alert("重置失败");
    }
  } catch (err) {
    console.error("Reset traffic error:", err);
    alert("重置失败");
  }
}

// --- Node Logic (Similar) ---
async function loadNodes() {
  const res = await fetch(NODE_API);
  nodes = await res.json();
  const container = document.getElementById("nodeTable");
  container.innerHTML = "";
  nodes.forEach((node) => {
    const card = document.createElement("div");
    card.className = "data-card";
    card.innerHTML = `
            <div class="card-header">
                <div class="card-title">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width:20px;height:20px;color:var(--text-secondary);">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
                    </svg>
                    ${node.name}
                </div>
                <span class="badge ${
                  node.enabled ? "badge-success" : "badge-danger"
                }">${node.enabled ? "正常" : "禁用"}</span>
            </div>

            <div class="card-row">
                <span>地址:</span>
                <span style="color: var(--text-secondary)">${node.host}</span>
            </div>
             <div class="card-row">
                <span>最后同步:</span>
                <span style="font-size: 12px; color: var(--text-secondary)">${
                  node.last_sync_at
                    ? new Date(node.last_sync_at).toLocaleString()
                    : "从未"
                }</span>
            </div>

            <div class="card-actions">
                <button class="btn btn-primary" style="padding: 6px 12px; font-size: 12px;" onclick="showEditNodeModal(${
                  node.id
                })">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 14px; height: 14px;">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                    </svg>
                    编辑
                </button>
                 <button class="btn ${
                   node.enabled ? "btn-danger" : "btn-success"
                 }" style="padding: 6px 12px; font-size: 12px;" onclick="toggleNode(${
      node.id
    }, ${!node.enabled}, '${node.name}', '${node.host}', '${node.secret}')">
                    ${node.enabled ? "禁用" : "启用"}
                </button>
                <button class="btn btn-danger" style="padding: 6px 12px; font-size: 12px;" onclick="deleteNode(${
                  node.id
                })">删除</button>
            </div>
        `;
    container.appendChild(card);
  });
}

async function saveNode() {
  const name = document.getElementById("nodeNameInput").value;
  const host = document.getElementById("nodeHostInput").value;
  const secret = document.getElementById("nodeSecretInput").value;
  if (!name || !host || !secret) return alert("请完整填写");
  try {
    if (editingNodeId) {
      // Update existing node - get current node to preserve enabled status
      const currentNode = nodes.find((node) => node.id === editingNodeId);
      await fetch(`${NODE_API}/${editingNodeId}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name,
          host,
          secret,
          enabled: currentNode ? currentNode.enabled : true,
        }),
      });
    } else {
      // Create new node
      await fetch(NODE_API, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, host, secret }),
      });
    }
    closeNodeModal();
    loadNodes();
  } catch (e) {
    alert("保存失败");
  }
}

async function deleteNode(id) {
  if (!confirm("删除节点将清除相关流量数据，确定吗？")) return;
  await fetch(`${NODE_API}/${id}`, { method: "DELETE" });
  loadNodes();
}

async function toggleNode(id, enabled, name, host, secret) {
  await fetch(`${NODE_API}/${id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name, host, secret, enabled }),
  });
  loadNodes();
}

// --- Modal Helpers ---
function showAddUserModal() {
  editingUserId = null;
  document.getElementById("userModalTitle").innerText = "添加用户";
  document.getElementById("userModal").classList.add("active");
  document.getElementById("usernameInput").value = "";
  document.getElementById("passwordInput").value = "";
  document.getElementById("trafficLimitInput").value = "0";
  document.getElementById("autoDisableInput").checked = true;
}

function showEditUserModal(id) {
  const user = users.find((u) => u.id === id);
  if (!user) return;

  editingUserId = id;
  document.getElementById("userModalTitle").innerText = "编辑用户";
  document.getElementById("userModal").classList.add("active");
  document.getElementById("usernameInput").value = user.username;
  document.getElementById("passwordInput").value = user.password;
  document.getElementById("trafficLimitInput").value = user.traffic_limit
    ? (user.traffic_limit / 1024 ** 3).toFixed(0)
    : "0";
  document.getElementById("autoDisableInput").checked =
    user.auto_disable_on_limit !== false;
}

function closeUserModal() {
  editingUserId = null;
  document.getElementById("userModal").classList.remove("active");
}

function showAddNodeModal() {
  editingNodeId = null;
  document.getElementById("nodeModalTitle").innerText = "添加节点";
  document.getElementById("nodeModal").classList.add("active");
  document.getElementById("nodeNameInput").value = "";
  document.getElementById("nodeHostInput").value = "";
  document.getElementById("nodeSecretInput").value = "";
}

function showEditNodeModal(id) {
  const node = nodes.find((n) => n.id === id);
  if (!node) return;

  editingNodeId = id;
  document.getElementById("nodeModalTitle").innerText = "编辑节点";
  document.getElementById("nodeModal").classList.add("active");
  document.getElementById("nodeNameInput").value = node.name;
  document.getElementById("nodeHostInput").value = node.host;
  document.getElementById("nodeSecretInput").value = node.secret;
}

function closeNodeModal() {
  editingNodeId = null;
  document.getElementById("nodeModal").classList.remove("active");
}

// Removed auto-refresh interval - only refresh on page load or user actions

async function refreshUsers() {
  await loadTrafficByNode();
  await loadUsers();
}

// --- Admin User Management ---
async function loadAdminUsers() {
  try {
    const res = await fetch(ADMIN_USER_API);
    adminUsers = await res.json();
    renderAdminUsers();
  } catch (err) {
    console.error("加载账号失败", err);
  }
}

function renderAdminUsers() {
  const container = document.getElementById("accountTable");
  container.innerHTML = "";
  
  adminUsers.forEach((user) => {
    const card = document.createElement("div");
    card.className = "data-card";
    
    const isDefaultAdmin = user.username === "admin";
    
    card.innerHTML = `
      <div class="card-header">
        <div class="card-title">
          <div style="width:32px; height:32px; background:#eff6ff; color:var(--primary); border-radius:50%; display:flex; align-items:center; justify-content:center; font-weight:800; font-size:14px;">
            ${user.username.charAt(0).toUpperCase()}
          </div>
          ${user.username}
        </div>
        <span class="badge ${user.role === 'admin' ? 'badge-success' : 'badge-primary'}">${user.role === 'admin' ? '管理员' : '普通用户'}</span>
      </div>
      
      <div class="card-row">
        <span>创建时间:</span>
        <span style="color: var(--text-secondary); font-size: 13px;">${new Date(user.created_at).toLocaleString()}</span>
      </div>
      
      <div class="card-actions">
        <button class="btn btn-primary" style="padding: 6px 12px; font-size: 12px;" onclick="showEditAdminUserModal(${user.id})">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 14px; height: 14px;">
            <path stroke-linecap="round" stroke-linejoin="round" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
          </svg>
          编辑
        </button>
        ${!isDefaultAdmin ? `
          <button class="btn btn-danger" style="padding: 6px 12px; font-size: 12px;" onclick="deleteAdminUser(${user.id})">删除</button>
        ` : '<span style="color: var(--text-secondary); font-size: 12px;">默认管理员不可删除</span>'}
      </div>
    `;
    container.appendChild(card);
  });
}

async function saveAdminUser() {
  const username = document.getElementById("adminUsernameInput").value;
  const password = document.getElementById("adminPasswordInput").value;
  const role = document.getElementById("adminRoleInput").value;

  if (!username) {
    alert("请输入用户名");
    return;
  }

  if (!editingAdminUserId && !password) {
    alert("请输入密码");
    return;
  }

  try {
    const payload = { username, role };
    if (password) {
      payload.password = password;
    }

    if (editingAdminUserId) {
      // Update existing admin user
      await fetch(`${ADMIN_USER_API}/${editingAdminUserId}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
    } else {
      // Create new admin user
      await fetch(ADMIN_USER_API, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
    }
    closeAdminUserModal();
    loadAdminUsers();
  } catch (e) {
    alert("操作失败");
  }
}

async function deleteAdminUser(id) {
  if (!confirm("确认删除该账号吗？")) return;
  
  try {
    const res = await fetch(`${ADMIN_USER_API}/${id}`, { method: "DELETE" });
    if (res.ok) {
      loadAdminUsers();
    } else {
      const data = await res.json();
      alert(data.message || "删除失败");
    }
  } catch (err) {
    alert("删除失败");
  }
}

function showAddAdminUserModal() {
  editingAdminUserId = null;
  document.getElementById("adminUserModalTitle").innerText = "添加账号";
  document.getElementById("adminUserModal").classList.add("active");
  document.getElementById("adminUsernameInput").value = "";
  document.getElementById("adminPasswordInput").value = "";
  document.getElementById("adminRoleInput").value = "user";
  document.getElementById("adminPasswordInput").placeholder = "请输入密码";
}

function showEditAdminUserModal(id) {
  const user = adminUsers.find((u) => u.id === id);
  if (!user) return;

  editingAdminUserId = id;
  document.getElementById("adminUserModalTitle").innerText = "编辑账号";
  document.getElementById("adminUserModal").classList.add("active");
  document.getElementById("adminUsernameInput").value = user.username;
  document.getElementById("adminPasswordInput").value = "";
  document.getElementById("adminRoleInput").value = user.role;
  document.getElementById("adminPasswordInput").placeholder = "留空则不修改密码";
}

function closeAdminUserModal() {
  editingAdminUserId = null;
  document.getElementById("adminUserModal").classList.remove("active");
}
