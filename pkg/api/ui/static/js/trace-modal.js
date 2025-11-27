(function () {
    function onReady(callback) {
        if (document.readyState === 'loading') {
            document.addEventListener('DOMContentLoaded', callback, { once: true });
        } else {
            callback();
        }
    }

    onReady(function initTraceModal() {
        const traceModal = document.getElementById('trace-modal');
        const traceBody = document.getElementById('trace-body');
        if (!traceModal || !traceBody) {
            return;
        }

        const TRACE_STAGE_LABELS = {
            blocklist: 'Blocklist',
            policy: 'Policy Engine',
            whitelist: 'Whitelist',
            rate_limit: 'Rate Limiter',
            cache: 'Cache'
        };

        const TRACE_ACTION_LABELS = {
            block: 'Blocked',
            allow: 'Allowed',
            redirect: 'Redirected',
            forward: 'Forwarded',
            blocked_hit: 'Cache Block',
            drop: 'Dropped'
        };

        function humanizeLabel(value) {
            if (!value) {
                return 'Unknown';
            }
            return value
                .toString()
                .replace(/[_-]+/g, ' ')
                .replace(/\b\w/g, function (char) {
                    return char.toUpperCase();
                });
        }

        function formatStageLabel(stage) {
            if (!stage) {
                return 'Unknown stage';
            }
            return TRACE_STAGE_LABELS[stage] || humanizeLabel(stage);
        }

        function formatActionLabel(action) {
            if (!action) {
                return '';
            }
            return TRACE_ACTION_LABELS[action] || humanizeLabel(action);
        }

        function createTraceChip(content, extraClass) {
            var cls = extraClass ? ' ' + extraClass : '';
            return '<span class="trace-chip' + cls + '">' + content + '</span>';
        }

        function chipClassForAction(action) {
            switch (action) {
                case 'block':
                case 'blocked_hit':
                case 'drop':
                    return 'trace-chip-danger';
                case 'allow':
                    return 'trace-chip-success';
                case 'redirect':
                    return 'trace-chip-warning';
                default:
                    return 'trace-chip-info';
            }
        }

        function renderTraceChips(entry) {
            const chips = [];
            if (entry.action) {
                chips.push(createTraceChip(formatActionLabel(entry.action), chipClassForAction(entry.action)));
            }
            if (entry.rule) {
                chips.push(createTraceChip('Rule: ' + entry.rule, 'trace-chip-info'));
            }
            if (entry.source) {
                chips.push(createTraceChip(entry.source, 'trace-chip-muted'));
            }
            return chips.join('');
        }

        function renderTraceMetadata(metadata) {
            if (!metadata || typeof metadata !== 'object') {
                return '';
            }
            const entries = Object.entries(metadata).filter(function ([key, value]) {
                return Boolean(key) && value !== undefined && value !== null && value !== '';
            });
            if (!entries.length) {
                return '';
            }
            const rows = entries
                .map(function ([key, value]) {
                    return (
                        '<div class="trace-meta-row">' +
                        '<div class="trace-meta-key">' + humanizeLabel(key) + '</div>' +
                        '<div class="trace-meta-value">' + value + '</div>' +
                        '</div>'
                    );
                })
                .join('');
            return '<div class="trace-metadata">' + rows + '</div>';
        }

        function renderTraceRow(entry) {
            const row = document.createElement('div');
            row.className = 'trace-row';
            const chips = renderTraceChips(entry);
            const metadata = renderTraceMetadata(entry.metadata);
            const detail = entry.detail ? '<p class="trace-detail">' + entry.detail + '</p>' : '';
            row.innerHTML =
                '<div class="trace-row-header">' +
                '<div>' +
                '<div class="trace-stage">' + formatStageLabel(entry.stage) + '</div>' +
                (entry.stage ? '<div class="trace-stage-meta">' + entry.stage + '</div>' : '') +
                '</div>' +
                (chips ? '<div class="trace-chips">' + chips + '</div>' : '') +
                '</div>' +
                detail +
                metadata;
            return row;
        }

        function openTraceModal(entries, domain, client) {
            traceBody.innerHTML = '';
            const header = document.createElement('div');
            header.className = 'trace-meta';
            header.innerHTML = '<strong>' + domain + '</strong><span>' + client + '</span>';
            traceBody.appendChild(header);

            if (!entries || !entries.length) {
                const empty = document.createElement('p');
                empty.className = 'text-muted';
                empty.textContent = 'No decision trace recorded.';
                traceBody.appendChild(empty);
            } else {
                entries.forEach(function (entry) {
                    traceBody.appendChild(renderTraceRow(entry));
                });
            }

            traceModal.style.display = 'flex';
        }

        function closeTraceModal() {
            traceModal.style.display = 'none';
        }

        document.body.addEventListener('click', function (event) {
            const target = event.target;
            if (!(target instanceof HTMLElement)) {
                return;
            }

            const trigger = target.closest('[data-trace-btn]');
            if (trigger) {
                try {
                    const entries = trigger.dataset.trace ? JSON.parse(trigger.dataset.trace) : [];
                    openTraceModal(entries, trigger.dataset.domain || 'unknown domain', trigger.dataset.client || 'unknown client');
                } catch (_) {
                    alert('Failed to parse trace payload');
                }
                return;
            }

            if (target === traceModal || target.closest('[data-trace-close]')) {
                closeTraceModal();
            }
        });

        window.GHTraceModal = {
            open: openTraceModal,
            close: closeTraceModal
        };
    });
})();
