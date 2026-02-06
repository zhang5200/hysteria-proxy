/**
 * Network Particle Background
 * Creates a canvas-based network connection effect.
 */

(function () {
    const canvas = document.getElementById('network-bg');
    if (!canvas) return;

    const ctx = canvas.getContext('2d');
    let width, height;
    let particles = [];

    // Configuration
    const config = {
        particleCount: 80,
        particleColor: 'rgba(15, 23, 42, 0.15)',
        lineColor: 'rgba(15, 23, 42, 0.15)',
        particleSize: 2,
        connectionDistance: 150,
        speed: 0.5
    };

    function updateThemeColors() {
        const style = getComputedStyle(document.documentElement);
        const particleColor = style.getPropertyValue('--particle-color').trim();
        const lineColor = style.getPropertyValue('--line-color').trim();
        const isDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
        config.particleColor = particleColor || (isDark ? 'rgba(125, 211, 252, 0.55)' : 'rgba(15, 23, 42, 0.15)');
        config.lineColor = lineColor || (isDark ? 'rgba(56, 189, 248, 0.35)' : 'rgba(15, 23, 42, 0.15)');
    }

    // Particle Class
    class Particle {
        constructor() {
            this.x = Math.random() * width;
            this.y = Math.random() * height;
            this.vx = (Math.random() - 0.5) * config.speed;
            this.vy = (Math.random() - 0.5) * config.speed;
            this.size = Math.random() * config.particleSize + 1;
        }

        update() {
            this.x += this.vx;
            this.y += this.vy;

            // Bounce off edges
            if (this.x < 0 || this.x > width) this.vx *= -1;
            if (this.y < 0 || this.y > height) this.vy *= -1;
        }

        draw() {
            ctx.beginPath();
            ctx.arc(this.x, this.y, this.size, 0, Math.PI * 2);
            ctx.fillStyle = config.particleColor;
            ctx.fill();
        }
    }

    // Initialize
    function init() {
        updateThemeColors();
        resize();
        createParticles();
        animate();
    }

    // Resize handling
    function resize() {
        width = canvas.width = window.innerWidth;
        height = canvas.height = window.innerHeight;
    }

    // Create particles
    function createParticles() {
        particles = [];
        const count = Math.floor((width * height) / 15000); // Responsive count
        const actualCount = Math.min(count, config.particleCount);
        
        for (let i = 0; i < actualCount; i++) {
            particles.push(new Particle());
        }
    }

    // Animation Loop
    function animate() {
        ctx.clearRect(0, 0, width, height);

        for (let i = 0; i < particles.length; i++) {
            const p = particles[i];
            p.update();
            p.draw();

            // Draw connections
            for (let j = i + 1; j < particles.length; j++) {
                const p2 = particles[j];
                const dx = p.x - p2.x;
                const dy = p.y - p2.y;
                const distance = Math.sqrt(dx * dx + dy * dy);

                if (distance < config.connectionDistance) {
                    ctx.beginPath();
                    ctx.strokeStyle = config.lineColor;
                    ctx.lineWidth = 1 - distance / config.connectionDistance;
                    ctx.moveTo(p.x, p.y);
                    ctx.lineTo(p2.x, p2.y);
                    ctx.stroke();
                }
            }
        }

        requestAnimationFrame(animate);
    }

    // Event Listeners
    window.addEventListener('resize', () => {
        resize();
        createParticles();
    });

    const media = window.matchMedia('(prefers-color-scheme: dark)');
    const handleThemeChange = () => updateThemeColors();
    if (media.addEventListener) {
        media.addEventListener('change', handleThemeChange);
    } else if (media.addListener) {
        media.addListener(handleThemeChange);
    }

    init();
})();
