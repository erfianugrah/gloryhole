/**
 * Focus Trap Utility
 * Traps keyboard focus within a modal/dialog for accessibility
 */
(function() {
    'use strict';

    const FOCUSABLE_SELECTOR = [
        'a[href]',
        'area[href]',
        'input:not([disabled]):not([type="hidden"])',
        'select:not([disabled])',
        'textarea:not([disabled])',
        'button:not([disabled])',
        '[tabindex]:not([tabindex="-1"])',
        '[contenteditable]'
    ].join(',');

    class FocusTrap {
        constructor(element) {
            this.element = element;
            this.previousFocus = null;
            this.handleKeyDown = this.handleKeyDown.bind(this);
        }

        /**
         * Activate the focus trap
         */
        activate() {
            // Store the element that had focus before opening
            this.previousFocus = document.activeElement;

            // Get all focusable elements
            this.updateFocusableElements();

            // Focus the first element
            if (this.focusableElements.length > 0) {
                this.focusableElements[0].focus();
            }

            // Add keyboard listener
            this.element.addEventListener('keydown', this.handleKeyDown);

            // Prevent body scroll
            document.body.style.overflow = 'hidden';
        }

        /**
         * Deactivate the focus trap
         */
        deactivate() {
            // Remove keyboard listener
            this.element.removeEventListener('keydown', this.handleKeyDown);

            // Restore body scroll
            document.body.style.overflow = '';

            // Restore focus to previous element
            if (this.previousFocus && this.previousFocus.focus) {
                this.previousFocus.focus();
            }

            this.previousFocus = null;
        }

        /**
         * Update the list of focusable elements
         */
        updateFocusableElements() {
            const nodes = this.element.querySelectorAll(FOCUSABLE_SELECTOR);
            this.focusableElements = Array.from(nodes).filter(node => {
                return node.offsetParent !== null && !node.hasAttribute('disabled');
            });
            this.firstElement = this.focusableElements[0];
            this.lastElement = this.focusableElements[this.focusableElements.length - 1];
        }

        /**
         * Handle keyboard events
         */
        handleKeyDown(event) {
            const isTab = event.key === 'Tab' || event.keyCode === 9;
            const isEscape = event.key === 'Escape' || event.keyCode === 27;

            if (isEscape) {
                // Close modal on Escape
                const closeButton = this.element.querySelector('[data-close-modal], [data-trace-close]');
                if (closeButton) {
                    closeButton.click();
                }
                return;
            }

            if (!isTab) {
                return;
            }

            // Update focusable elements in case DOM changed
            this.updateFocusableElements();

            if (this.focusableElements.length === 0) {
                event.preventDefault();
                return;
            }

            if (event.shiftKey) {
                // Shift + Tab
                if (document.activeElement === this.firstElement) {
                    event.preventDefault();
                    this.lastElement.focus();
                }
            } else {
                // Tab
                if (document.activeElement === this.lastElement) {
                    event.preventDefault();
                    this.firstElement.focus();
                }
            }
        }
    }

    /**
     * Auto-initialize focus traps for modals
     */
    function initializeFocusTraps() {
        const modals = document.querySelectorAll('.modal');
        const traps = new Map();

        modals.forEach(modal => {
            const trap = new FocusTrap(modal);
            traps.set(modal.id, trap);

            // Watch for display changes using MutationObserver
            const observer = new MutationObserver((mutations) => {
                mutations.forEach((mutation) => {
                    if (mutation.type === 'attributes' && mutation.attributeName === 'style') {
                        const display = window.getComputedStyle(modal).display;
                        if (display === 'flex' || display === 'block') {
                            // Modal is now visible
                            setTimeout(() => trap.activate(), 10);
                        } else if (display === 'none') {
                            // Modal is now hidden
                            trap.deactivate();
                        }
                    }
                });
            });

            observer.observe(modal, {
                attributes: true,
                attributeFilter: ['style']
            });
        });

        // Alternative: Use a custom event system
        document.addEventListener('modal:open', (event) => {
            const modalId = event.detail && event.detail.modalId;
            if (modalId && traps.has(modalId)) {
                traps.get(modalId).activate();
            }
        });

        document.addEventListener('modal:close', (event) => {
            const modalId = event.detail && event.detail.modalId;
            if (modalId && traps.has(modalId)) {
                traps.get(modalId).deactivate();
            }
        });
    }

    // Initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initializeFocusTraps);
    } else {
        initializeFocusTraps();
    }

    // Export for manual use
    window.FocusTrap = FocusTrap;
})();
