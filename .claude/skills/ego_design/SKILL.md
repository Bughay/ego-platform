---
name: EGO Design System
description: Updating files with `.html`, `.css`, or `.js` file in the `frontend/` directory. Use whenever creating, modifying, or reviewing HTML/CSS for the EGO platform.
---

# EGO Design System

You are building for EGO — a cyberpunk-themed authentication and application platform. Every page and component must follow this design language rigorously. Never deviate from these rules unless explicitly instructed.

## Triggers

Apply this design system when:
- Updating files with `.html`, `.css`, or `.js` file in the `frontend/` directory
- Adding a new page, card, form, button, or UI component
- Styling anything visible to the user
- Asked to "make it look like EGO" or "use the EGO design"
- Reviewing UI code for consistency

## Color Palette (CSS Custom Properties)

Always define these variables on `:root`. Never hardcode color hex values.

```css
:root {
  --cyan:    #00f5ff;
  --magenta: #ff00ff;
  --dark:    #040d14;
  --card-bg: rgba(4, 18, 30, 0.75);
  --border:  rgba(0, 245, 255, 0.35);
  --text:    #c8f0ff;
  --muted:   rgba(0, 245, 255, 0.45);
  --error:   #ff4d6d;
}

*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

:root {
  --cyan:    #00f5ff;
  --magenta: #ff00ff;
  --dark:    #040d14;
  --card-bg: rgba(4, 18, 30, 0.75);
  --border:  rgba(0, 245, 255, 0.35);
  --text:    #c8f0ff;
  --muted:   rgba(0, 245, 255, 0.45);
  --error:   #ff4d6d;
}

/* ── Background ── */
body {
  min-height: 100vh;
  display: grid;
  place-items: center;
  background: var(--dark);
  font-family: 'Share Tech Mono', monospace;
  color: var(--text);
  overflow: hidden;
  position: relative;
}

/* Scanline overlay */
body::before {
  content: '';
  position: fixed;
  inset: 0;
  background: repeating-linear-gradient(
    0deg,
    transparent,
    transparent 2px,
    rgba(0, 0, 0, 0.18) 2px,
    rgba(0, 0, 0, 0.18) 4px
  );
  pointer-events: none;
  z-index: 10;
}

/* Animated orbs */
canvas#bg {
  position: fixed;
  inset: 0;
  z-index: 0;
}

/* ── Card ── */
.card {
  position: relative;
  z-index: 1;
  width: min(420px, 92vw);
  padding: 48px 40px 40px;
  background: var(--card-bg);
  border: 1px solid var(--border);
  border-radius: 4px;
  backdrop-filter: blur(18px);
  -webkit-backdrop-filter: blur(18px);
  box-shadow:
    0 0 30px rgba(0, 245, 255, 0.08),
    0 0 80px rgba(0, 245, 255, 0.04),
    inset 0 0 30px rgba(0, 245, 255, 0.03);
  overflow: hidden;
}

/* Corner accents */
.card::before,
.card::after {
  content: '';
  position: absolute;
  width: 16px;
  height: 16px;
  border-color: var(--cyan);
  border-style: solid;
}
.card::before { top: -1px; left: -1px; border-width: 2px 0 0 2px; }
.card::after  { bottom: -1px; right: -1px; border-width: 0 2px 2px 0; }

/* ── Buttons ── */
.btn {
  width: 100%;
  padding: 13px;
  font-family: 'Orbitron', sans-serif;
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 4px;
  border-radius: 2px;
  cursor: pointer;
  transition: transform 0.15s, box-shadow 0.25s, background 0.25s;
  position: relative;
  overflow: hidden;
}

.btn::after {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(120deg, transparent 30%, rgba(255,255,255,0.08) 50%, transparent 70%);
  transform: translateX(-100%);
  transition: transform 0.4s;
}
.btn:hover::after { transform: translateX(100%); }

.btn-primary {
  background: var(--cyan);
  color: #020c12;
  border: 1px solid var(--cyan);
  box-shadow: 0 0 18px rgba(0, 245, 255, 0.35), inset 0 0 12px rgba(0, 245, 255, 0.1);
}
.btn-primary:hover {
  transform: translateY(-2px);
  box-shadow: 0 0 32px rgba(0, 245, 255, 0.65), 0 6px 20px rgba(0, 245, 255, 0.25);
}
.btn-primary:active { transform: translateY(0); }

.btn-outline {
  background: transparent;
  color: var(--cyan);
  border: 1px solid rgba(0, 245, 255, 0.5);
  box-shadow: 0 0 10px rgba(0, 245, 255, 0.1);
}
.btn-outline:hover {
  transform: translateY(-2px);
  border-color: var(--magenta);
  color: var(--magenta);
  box-shadow: 0 0 24px rgba(255, 0, 255, 0.35), 0 6px 16px rgba(255, 0, 255, 0.15);
}
.btn-outline:active { transform: translateY(0); }