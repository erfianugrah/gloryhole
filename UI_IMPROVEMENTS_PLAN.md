# UI Improvements Plan

**Branch:** `ui-improvements`
**Date:** 2025-12-10
**Objective:** Enhance accessibility, maintainability, and user experience of the Gloryhole DNS UI

---

## Phase 1: Quick Wins (High Impact, Low Effort)

### 1.1 Reduced Motion Support ⚡ (Priority: High)
**Time:** 15 minutes
**File:** `pkg/api/ui/static/css/style.css`

**Changes:**
- Add `@media (prefers-reduced-motion: reduce)` queries
- Disable scan animations on stat cards
- Reduce animation durations
- Remove auto-playing effects for accessibility

**Impact:** Improved accessibility for users with vestibular disorders

---

### 1.2 Color Contrast Fixes ⚡ (Priority: High)
**Time:** 30 minutes
**File:** `pkg/api/ui/static/css/style.css`

**Changes:**
- Update `--text-muted` from `#7d8cc4` to `#9ca8d4`
- Verify WCAG AA compliance (4.5:1 for normal text, 3:1 for large text)
- Test all text on background combinations
- Update status chip colors if needed

**Impact:** Better readability for all users

---

### 1.3 Chart Accessibility ⚡ (Priority: High)
**Time:** 1 hour
**Files:**
- `pkg/api/ui/templates/dashboard.html`

**Changes:**
- Add `aria-describedby` linking charts to legends
- Add `role="img"` and `aria-label` to canvas elements
- Ensure legend items have proper keyboard navigation
- Add screen reader text for chart data summaries

**Impact:** Charts become accessible to screen reader users

---

## Phase 2: Modal & Focus Management (Medium Effort)

### 2.1 Focus Trap Implementation ⚡ (Priority: High)
**Time:** 1-2 hours
**Files:**
- `pkg/api/ui/static/js/focus-trap.js` (new)
- `pkg/api/ui/templates/base.html`
- `pkg/api/ui/templates/clients.html`

**Changes:**
- Create reusable focus trap utility
- Trap focus within modals when open
- Restore focus to trigger element on close
- Handle Escape key to close modal
- Prevent body scroll when modal is open

**Implementation:**
```javascript
// New file: static/js/focus-trap.js
class FocusTrap {
  constructor(element) {
    this.element = element;
    this.previousFocus = null;
  }

  activate() {
    this.previousFocus = document.activeElement;
    // Implementation...
  }

  deactivate() {
    if (this.previousFocus) {
      this.previousFocus.focus();
    }
  }
}
```

**Impact:** Improved keyboard navigation and accessibility

---

## Phase 3: Form Validation Enhancement (Medium Effort)

### 3.1 Inline Validation Feedback ⚡ (Priority: Medium)
**Time:** 2-3 hours
**Files:**
- `pkg/api/ui/static/css/style.css` (form validation styles)
- `pkg/api/ui/templates/settings.html`
- `pkg/api/ui/static/js/form-validation.js` (new)

**Changes:**
- Add `.form-error` and `.form-success` classes
- Implement real-time validation for:
  - DNS server addresses (IP:port format)
  - Email addresses
  - Duration strings (e.g., "30s", "5m")
  - Number ranges
- Use `aria-invalid` and `aria-describedby` for errors
- Show inline error messages instead of alerts

**Example:**
```html
<div class="form-group">
  <label for="servers">Resolvers</label>
  <textarea id="servers" aria-describedby="servers-error"></textarea>
  <span id="servers-error" class="form-error" hidden></span>
</div>
```

**Impact:** Better user experience with immediate feedback

---

## Phase 4: JavaScript Organization (Higher Effort)

### 4.1 Extract Inline Scripts ⚡ (Priority: Medium)
**Time:** 4-6 hours
**Files:**
- `pkg/api/ui/static/js/modules/charts.js` (new)
- `pkg/api/ui/static/js/modules/clients.js` (new)
- `pkg/api/ui/static/js/modules/settings.js` (new)
- `pkg/api/ui/static/js/modules/pagination.js` (new)
- `pkg/api/ui/templates/dashboard.html` (refactor)
- `pkg/api/ui/templates/clients.html` (refactor)
- `pkg/api/ui/templates/settings.html` (refactor)

**Structure:**
```
pkg/api/ui/static/js/
├── modules/
│   ├── charts.js         (Chart initialization & updates)
│   ├── clients.js        (Client management logic)
│   ├── settings.js       (Settings page logic)
│   ├── pagination.js     (Reusable pagination)
│   ├── modal.js          (Modal utilities)
│   └── focus-trap.js     (Focus management)
├── utils/
│   ├── api.js            (API helper functions)
│   └── validators.js     (Form validation)
└── trace-modal.js        (existing)
```

**Changes:**
- Extract dashboard chart code to `charts.js`
- Extract client table code to `clients.js`
- Extract settings functions to `settings.js`
- Use ES6 modules with proper imports/exports
- Add module bundling or use native ES modules

**Impact:** Better maintainability and code reusability

---

## Phase 5: CSS Architecture (Higher Effort)

### 5.1 Split CSS into Modules (Priority: Low)
**Time:** 3-4 hours
**Files:**
- `pkg/api/ui/static/css/base/` (new directory)
- `pkg/api/ui/static/css/components/` (new directory)
- `pkg/api/ui/static/css/pages/` (new directory)

**Structure:**
```
pkg/api/ui/static/css/
├── base/
│   ├── variables.css     (CSS custom properties)
│   ├── reset.css         (Base resets)
│   ├── typography.css    (Font families, sizes)
│   └── animations.css    (Keyframes)
├── components/
│   ├── buttons.css
│   ├── cards.css
│   ├── forms.css
│   ├── modal.css
│   ├── nav.css
│   ├── tables.css
│   └── status-chips.css
├── pages/
│   ├── dashboard.css
│   ├── clients.css
│   └── settings.css
└── style.css             (Import all)
```

**Impact:** Better organization and easier maintenance

---

## Phase 6: UX Enhancements (Optional)

### 6.1 Toast Notification System (Priority: Low)
**Time:** 2-3 hours
**Files:**
- `pkg/api/ui/static/js/toast.js` (new)
- `pkg/api/ui/static/css/components/toast.css` (new)
- `pkg/api/ui/templates/base.html` (add toast container)

**Features:**
- Success, error, warning, info types
- Auto-dismiss with configurable timeout
- Stack multiple toasts
- Accessible with ARIA live regions
- Close button
- Smooth animations

**Usage:**
```javascript
import { toast } from './toast.js';

toast.success('Settings saved successfully');
toast.error('Failed to update DNS servers');
```

**Impact:** Better feedback than alert() dialogs

---

### 6.2 Empty State Improvements (Priority: Low)
**Time:** 1-2 hours
**Files:**
- `pkg/api/ui/templates/*.html`
- `pkg/api/ui/static/css/components/empty-state.css`

**Changes:**
- Add illustrations (SVG icons)
- Provide actionable guidance
- Add "Getting Started" buttons
- Link to documentation

**Impact:** Better first-time user experience

---

### 6.3 Loading State Enhancements (Priority: Low)
**Time:** 1 hour
**Files:**
- `pkg/api/ui/static/css/style.css`

**Changes:**
- Show stale data with reduced opacity during refresh
- Add subtle pulse to loading indicators
- Improve skeleton loader styling

**Impact:** Better perceived performance

---

## Implementation Order

### Sprint 1: Accessibility Core (Day 1)
1. ✅ Reduced motion support (15 min)
2. ✅ Color contrast fixes (30 min)
3. ✅ Chart accessibility (1 hour)
4. ✅ Focus trap implementation (2 hours)

**Total: ~4 hours**

### Sprint 2: Forms & Validation (Day 2)
5. ✅ Inline form validation (3 hours)
6. ✅ Form validation styles (30 min)

**Total: ~3.5 hours**

### Sprint 3: Code Organization (Day 3-4)
7. ✅ Extract inline scripts to modules (6 hours)
8. ⚠️ CSS architecture refactor (optional, 4 hours)

**Total: 6-10 hours**

### Sprint 4: Polish (Optional)
9. Toast notification system (3 hours)
10. Empty state improvements (2 hours)
11. Loading enhancements (1 hour)

**Total: ~6 hours**

---

## Testing Checklist

### Accessibility Testing
- [ ] Screen reader testing (NVDA/JAWS on Windows, VoiceOver on Mac)
- [ ] Keyboard-only navigation
- [ ] Color contrast verification (WebAIM Contrast Checker)
- [ ] Reduced motion testing
- [ ] Focus visible on all interactive elements

### Browser Testing
- [ ] Chrome/Edge (latest)
- [ ] Firefox (latest)
- [ ] Safari (latest)
- [ ] Mobile Chrome
- [ ] Mobile Safari

### Functionality Testing
- [ ] Modal open/close with keyboard
- [ ] Form validation displays correctly
- [ ] Charts render and update properly
- [ ] HTMX partial updates work
- [ ] Theme toggle functions
- [ ] Responsive layouts at all breakpoints

---

## Success Metrics

### Accessibility
- WCAG 2.1 AA compliance
- Lighthouse accessibility score > 95
- Zero critical accessibility violations in axe DevTools

### Performance
- No performance regression
- Lighthouse performance score maintained or improved
- Bundle size increase < 10%

### Code Quality
- JavaScript extracted from templates
- Linter warnings = 0
- Consistent code style

---

## Rollback Plan

If critical issues are found:
1. Revert to `main` branch
2. Cherry-pick working commits
3. Create hotfix branch for issues
4. Re-test before merging

---

## Notes

- All changes should maintain backward compatibility
- No breaking changes to existing functionality
- Server-side templates remain as-is (minimal changes)
- Focus on client-side improvements only
- Keep the "surveillance terminal" aesthetic intact
