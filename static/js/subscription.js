// Subscription management functionality

// Cache for subscriptions data
let cachedSubscriptions = null;

// Load subscriptions for all users
async function loadSubscriptions() {
    try {
        const res = await fetch(USER_API);
        const users = await res.json();
        
        // Get subscription URLs for all users
        const newSubscriptions = await Promise.all(users.map(async (user) => {
            let subscriptionUrl = '';
            try {
                const res = await fetch(`${USER_API}/${user.id}/subscription`);
                const data = await res.json();
                subscriptionUrl = data.url || '';
            } catch (err) {
                console.error(`获取用户 ${user.username} 订阅链接失败`, err);
            }
            return { 
                userId: user.id, 
                username: user.username, 
                enabled: user.enabled,
                subscriptionUrl 
            };
        }));
        
        // Check if data has changed
        if (hasSubscriptionDataChanged(newSubscriptions)) {
            cachedSubscriptions = newSubscriptions;
            renderSubscriptions(users, newSubscriptions);
        }
    } catch (err) {
        console.error("加载订阅失败", err);
    }
}

// Check if subscription data has changed
function hasSubscriptionDataChanged(newSubscriptions) {
    if (!cachedSubscriptions) return true;
    if (cachedSubscriptions.length !== newSubscriptions.length) return true;
    
    for (let i = 0; i < newSubscriptions.length; i++) {
        const cached = cachedSubscriptions.find(s => s.userId === newSubscriptions[i].userId);
        if (!cached) return true;
        if (cached.subscriptionUrl !== newSubscriptions[i].subscriptionUrl) return true;
        if (cached.username !== newSubscriptions[i].username) return true;
        if (cached.enabled !== newSubscriptions[i].enabled) return true;
    }
    
    return false;
}

// Render subscription cards
async function renderSubscriptions(users, userCards) {
    const container = document.getElementById("subscriptionTable");
    container.innerHTML = "";

    // Render all cards
    userCards.forEach(({ userId, username, enabled, subscriptionUrl }) => {
        const user = users.find(u => u.id === userId);
        if (!user) return;

        const card = document.createElement("div");
        card.className = "data-card";

        card.innerHTML = `
            <div class="card-header">
                <div class="card-title">
                    <div style="width:32px; height:32px; background:#eff6ff; color:var(--primary); border-radius:50%; display:flex; align-items:center; justify-content:center; font-weight:800; font-size:14px;">
                        ${username.charAt(0).toUpperCase()}
                    </div>
                    ${username}
                </div>
                <span class="badge ${enabled ? 'badge-success' : 'badge-danger'}">${enabled ? '启用' : '禁用'}</span>
            </div>
            
            <div style="margin-top: 12px;">
                <label style="font-size: 13px; font-weight: 600; color: var(--text-main); margin-bottom: 8px;">订阅链接</label>
                <div class="subscription-url">
                    <input type="text" readonly value="${subscriptionUrl}" id="sub-url-${userId}" style="font-size: 11px;">
                    <button class="btn btn-primary" style="padding: 8px 12px; font-size: 12px;" onclick="copySubscriptionUrl(${userId})">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 14px; height: 14px;">
                            <path stroke-linecap="round" stroke-linejoin="round" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                        </svg>
                        复制
                    </button>
                </div>
            </div>
            
            <div class="qr-code-container" id="qr-${userId}">
                <div style="font-size: 13px; font-weight: 600; margin-bottom: 10px; color: var(--text-main);">扫描二维码订阅</div>
                <div id="qr-canvas-${userId}" style="display: inline-block;"></div>
            </div>
            
            <div class="card-actions">
                <button class="btn btn-success" style="padding: 6px 12px; font-size: 12px;" onclick="regenerateToken(${userId})">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 14px; height: 14px;">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                    </svg>
                    重新生成链接
                </button>
            </div>
        `;

        container.appendChild(card);

        // Generate QR code after DOM is ready
        if (subscriptionUrl) {
            // Use setTimeout to ensure DOM is fully rendered
            setTimeout(() => {
                generateQRCode(`qr-canvas-${userId}`, subscriptionUrl);
            }, 100);
        }
    });
}

// Copy subscription URL to clipboard
async function copySubscriptionUrl(userId) {
    const input = document.getElementById(`sub-url-${userId}`);
    try {
        await navigator.clipboard.writeText(input.value);
        showCopyFeedback();
    } catch (err) {
        // Fallback for older browsers
        input.select();
        document.execCommand('copy');
        showCopyFeedback();
    }
}

// Show copy feedback
function showCopyFeedback() {
    const feedback = document.createElement('div');
    feedback.className = 'copy-feedback';
    feedback.textContent = '✓ 已复制到剪贴板';
    document.body.appendChild(feedback);

    setTimeout(() => {
        feedback.style.opacity = '0';
        setTimeout(() => feedback.remove(), 300);
    }, 2000);
}

// Generate QR code using qrcodejs
function generateQRCode(canvasId, text) {
    const container = document.getElementById(canvasId);
    if (!container || !text) return;

    try {
        // Clear previous QR code if exists
        container.innerHTML = '';
        
        // Generate QR code
        new QRCode(container, {
            text: text,
            width: 200,
            height: 200,
            colorDark: '#111827',
            colorLight: '#ffffff',
            correctLevel: QRCode.CorrectLevel.M
        });
    } catch (err) {
        console.error('QR Code error:', err);
    }
}

// Regenerate subscription token
async function regenerateToken(userId) {
    if (!confirm('重新生成订阅链接将使旧链接失效，确定继续吗？')) return;

    try {
        const res = await fetch(`${USER_API}/${userId}/generate-token`, {
            method: 'POST'
        });

        if (res.ok) {
            alert('订阅链接已重新生成');
            // Clear cache to force regeneration
            cachedSubscriptions = null;
            loadSubscriptions();
        } else {
            alert('生成失败');
        }
    } catch (err) {
        console.error('Token generation error:', err);
        alert('生成失败');
    }
}
