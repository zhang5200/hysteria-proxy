// Subscription management functionality

// Load subscriptions for all users
async function loadSubscriptions() {
    try {
        const res = await fetch(USER_API);
        const users = await res.json();
        renderSubscriptions(users);
    } catch (err) {
        console.error("加载订阅失败", err);
    }
}

// Render subscription cards
function renderSubscriptions(users) {
    const container = document.getElementById("subscriptionTable");
    container.innerHTML = "";

    users.forEach(async (user) => {
        const card = document.createElement("div");
        card.className = "data-card";

        // Get subscription URL
        let subscriptionUrl = '';
        try {
            const res = await fetch(`${USER_API}/${user.id}/subscription`);
            const data = await res.json();
            subscriptionUrl = data.url || '';
        } catch (err) {
            console.error(`获取用户 ${user.username} 订阅链接失败`, err);
        }

        card.innerHTML = `
            <div class="card-header">
                <div class="card-title">
                    <div style="width:32px; height:32px; background:#eff6ff; color:var(--primary); border-radius:50%; display:flex; align-items:center; justify-content:center; font-weight:800; font-size:14px;">
                        ${user.username.charAt(0).toUpperCase()}
                    </div>
                    ${user.username}
                </div>
                <span class="badge ${user.enabled ? 'badge-success' : 'badge-danger'}">${user.enabled ? '启用' : '禁用'}</span>
            </div>
            
            <div style="margin-top: 12px;">
                <label style="font-size: 13px; font-weight: 600; color: var(--text-main); margin-bottom: 8px;">订阅链接</label>
                <div class="subscription-url">
                    <input type="text" readonly value="${subscriptionUrl}" id="sub-url-${user.id}" style="font-size: 11px;">
                    <button class="btn btn-primary" style="padding: 8px 12px; font-size: 12px;" onclick="copySubscriptionUrl(${user.id})">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 14px; height: 14px;">
                            <path stroke-linecap="round" stroke-linejoin="round" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                        </svg>
                        复制
                    </button>
                </div>
            </div>
            
            <div class="qr-code-container" id="qr-${user.id}">
                <div style="font-size: 13px; font-weight: 600; margin-bottom: 10px; color: var(--text-main);">扫描二维码订阅</div>
                <canvas id="qr-canvas-${user.id}"></canvas>
            </div>
            
            <div class="card-actions">
                <button class="btn btn-success" style="padding: 6px 12px; font-size: 12px;" onclick="regenerateToken(${user.id})">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 14px; height: 14px;">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                    </svg>
                    重新生成链接
                </button>
            </div>
        `;

        container.appendChild(card);

        // Generate QR code if URL exists
        if (subscriptionUrl) {
            generateQRCode(`qr-canvas-${user.id}`, subscriptionUrl);
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

// Generate QR code using qrcode.js
function generateQRCode(canvasId, text) {
    const canvas = document.getElementById(canvasId);
    if (!canvas || !text) return;

    try {
        QRCode.toCanvas(canvas, text, {
            width: 200,
            margin: 2,
            color: {
                dark: '#111827',
                light: '#ffffff'
            }
        }, function (error) {
            if (error) console.error('QR Code generation error:', error);
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
            loadSubscriptions();
        } else {
            alert('生成失败');
        }
    } catch (err) {
        console.error('Token generation error:', err);
        alert('生成失败');
    }
}
