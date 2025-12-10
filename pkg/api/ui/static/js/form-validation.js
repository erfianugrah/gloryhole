/**
 * Form Validation Utility
 * Provides real-time inline validation for forms
 */
(function() {
    'use strict';

    /**
     * Validation rules
     */
    const validators = {
        /**
         * Validate DNS server address (IP:port format)
         * Supports IPv4 and IPv6 with optional port
         */
        dnsServer: function(value) {
            if (!value || !value.trim()) {
                return { valid: false, message: 'DNS server address is required' };
            }

            const lines = value.split('\n').filter(line => line.trim());
            for (const line of lines) {
                const trimmed = line.trim();

                // IPv4:port or IPv6:port pattern
                const ipv4Pattern = /^(\d{1,3}\.){3}\d{1,3}:\d{1,5}$/;
                const ipv6Pattern = /^\[[\da-fA-F:]+\]:\d{1,5}$/;

                if (!ipv4Pattern.test(trimmed) && !ipv6Pattern.test(trimmed)) {
                    return {
                        valid: false,
                        message: `Invalid DNS server format: ${trimmed}. Use IP:port format (e.g., 1.1.1.1:53 or [2606:4700:4700::1111]:53)`
                    };
                }

                // Validate port number
                const portMatch = trimmed.match(/:(\d{1,5})$/);
                if (portMatch) {
                    const port = parseInt(portMatch[1], 10);
                    if (port < 1 || port > 65535) {
                        return {
                            valid: false,
                            message: `Invalid port number ${port}. Must be between 1 and 65535`
                        };
                    }
                }
            }

            return { valid: true };
        },

        /**
         * Validate email address
         */
        email: function(value) {
            if (!value || !value.trim()) {
                return { valid: true }; // Empty is OK if not required
            }

            const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
            if (!emailPattern.test(value.trim())) {
                return { valid: false, message: 'Invalid email address format' };
            }

            return { valid: true };
        },

        /**
         * Validate Go duration string (e.g., "30s", "5m", "24h")
         */
        duration: function(value) {
            if (!value || !value.trim()) {
                return { valid: false, message: 'Duration is required' };
            }

            const durationPattern = /^\d+(\.\d+)?(ns|us|µs|ms|s|m|h)$/;
            if (!durationPattern.test(value.trim())) {
                return {
                    valid: false,
                    message: 'Invalid duration format. Use Go duration (e.g., 30s, 5m, 24h)'
                };
            }

            return { valid: true };
        },

        /**
         * Validate number within range
         */
        numberRange: function(value, min, max) {
            if (!value && value !== 0) {
                return { valid: false, message: 'Value is required' };
            }

            const num = parseFloat(value);
            if (isNaN(num)) {
                return { valid: false, message: 'Must be a valid number' };
            }

            if (min !== undefined && num < min) {
                return { valid: false, message: `Must be at least ${min}` };
            }

            if (max !== undefined && num > max) {
                return { valid: false, message: `Must be at most ${max}` };
            }

            return { valid: true };
        },

        /**
         * Validate required field
         */
        required: function(value) {
            if (!value || !value.toString().trim()) {
                return { valid: false, message: 'This field is required' };
            }
            return { valid: true };
        }
    };

    /**
     * Show validation error for an input
     */
    function showError(input, message) {
        const formGroup = input.closest('.form-group');
        if (!formGroup) return;

        // Set aria-invalid
        input.setAttribute('aria-invalid', 'true');
        input.classList.add('is-invalid');
        input.classList.remove('is-valid');

        // Find or create error message element
        const errorId = input.id + '-error';
        let errorElement = document.getElementById(errorId);

        if (!errorElement) {
            errorElement = document.createElement('span');
            errorElement.id = errorId;
            errorElement.className = 'form-error';
            errorElement.setAttribute('role', 'alert');

            // Insert after the input or after help text
            const helpText = formGroup.querySelector('.form-help');
            if (helpText) {
                helpText.parentNode.insertBefore(errorElement, helpText.nextSibling);
            } else {
                input.parentNode.insertBefore(errorElement, input.nextSibling);
            }
        }

        errorElement.textContent = message;
        errorElement.removeAttribute('hidden');

        // Link error to input
        input.setAttribute('aria-describedby', errorId);
    }

    /**
     * Show validation success for an input
     */
    function showSuccess(input) {
        const formGroup = input.closest('.form-group');
        if (!formGroup) return;

        // Set aria-invalid
        input.setAttribute('aria-invalid', 'false');
        input.classList.add('is-valid');
        input.classList.remove('is-invalid');

        // Hide error message
        const errorId = input.id + '-error';
        const errorElement = document.getElementById(errorId);
        if (errorElement) {
            errorElement.setAttribute('hidden', '');
            errorElement.textContent = '';
        }

        // Remove aria-describedby if only pointing to error
        const describedBy = input.getAttribute('aria-describedby');
        if (describedBy === errorId) {
            input.removeAttribute('aria-describedby');
        }
    }

    /**
     * Clear validation state for an input
     */
    function clearValidation(input) {
        input.classList.remove('is-valid', 'is-invalid');
        input.removeAttribute('aria-invalid');

        const errorId = input.id + '-error';
        const errorElement = document.getElementById(errorId);
        if (errorElement) {
            errorElement.setAttribute('hidden', '');
            errorElement.textContent = '';
        }
    }

    /**
     * Validate an input based on data attributes
     */
    function validateInput(input) {
        const value = input.value;
        const validationType = input.getAttribute('data-validate');

        if (!validationType) {
            return true;
        }

        let result;

        switch (validationType) {
            case 'dns-server':
                result = validators.dnsServer(value);
                break;

            case 'email':
                result = validators.email(value);
                break;

            case 'duration':
                result = validators.duration(value);
                break;

            case 'number':
                const min = input.getAttribute('data-min');
                const max = input.getAttribute('data-max');
                result = validators.numberRange(
                    value,
                    min ? parseFloat(min) : undefined,
                    max ? parseFloat(max) : undefined
                );
                break;

            case 'required':
                result = validators.required(value);
                break;

            default:
                return true;
        }

        if (result.valid) {
            showSuccess(input);
            return true;
        } else {
            showError(input, result.message);
            return false;
        }
    }

    /**
     * Validate all inputs in a form
     */
    function validateForm(form) {
        const inputs = form.querySelectorAll('[data-validate]');
        let isValid = true;

        inputs.forEach(input => {
            if (!validateInput(input)) {
                isValid = false;
            }
        });

        return isValid;
    }

    /**
     * Initialize form validation
     */
    function initializeValidation() {
        // Real-time validation on blur
        document.addEventListener('blur', function(event) {
            const input = event.target;
            if (input.hasAttribute('data-validate')) {
                validateInput(input);
            }
        }, true);

        // Clear validation on focus
        document.addEventListener('focus', function(event) {
            const input = event.target;
            if (input.hasAttribute('data-validate')) {
                // Only clear if it was invalid
                if (input.classList.contains('is-invalid')) {
                    clearValidation(input);
                }
            }
        }, true);

        // Validate on form submit
        document.addEventListener('submit', function(event) {
            const form = event.target;
            const hasValidation = form.querySelector('[data-validate]');

            if (hasValidation && !validateForm(form)) {
                event.preventDefault();

                // Focus first invalid input
                const firstInvalid = form.querySelector('.is-invalid');
                if (firstInvalid) {
                    firstInvalid.focus();
                }
            }
        });

        // Debounced validation on input (for better UX)
        let inputTimer;
        document.addEventListener('input', function(event) {
            const input = event.target;
            if (input.hasAttribute('data-validate')) {
                clearTimeout(inputTimer);
                inputTimer = setTimeout(() => {
                    // Only validate if the input has been touched (blurred at least once)
                    if (input.classList.contains('is-valid') || input.classList.contains('is-invalid')) {
                        validateInput(input);
                    }
                }, 500);
            }
        });
    }

    // Initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initializeValidation);
    } else {
        initializeValidation();
    }

    // Export for manual use
    window.FormValidation = {
        validate: validateInput,
        validateForm: validateForm,
        showError: showError,
        showSuccess: showSuccess,
        clearValidation: clearValidation,
        validators: validators
    };
})();
