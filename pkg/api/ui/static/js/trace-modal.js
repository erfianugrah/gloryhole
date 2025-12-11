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
            const chip = document.createElement('span');
            chip.className = 'trace-chip' + (extraClass ? ' ' + extraClass : '');
            chip.textContent = content;
            return chip;
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
            const elements = [];
            if (entry.action) {
                elements.push(createTraceChip(formatActionLabel(entry.action), chipClassForAction(entry.action)));
            }
            if (entry.rule) {
                elements.push(createTraceChip('Rule: ' + String(entry.rule), 'trace-chip-info'));
            }
            if (entry.source) {
                elements.push(createTraceChip(String(entry.source), 'trace-chip-muted'));
            }
            if (!elements.length) {
                return null;
            }
            const container = document.createElement('div');
            container.className = 'trace-chips';
            elements.forEach(function (chip) {
                container.appendChild(chip);
            });
            return container;
        }

        function renderTraceMetadata(metadata) {
            if (!metadata || typeof metadata !== 'object') {
                return null;
            }
            const entries = Object.entries(metadata).filter(function ([key, value]) {
                return Boolean(key) && value !== undefined && value !== null && value !== '';
            });
            if (!entries.length) {
                return null;
            }

            // Reorder to show pattern and lists first
            const priorityKeys = ['pattern', 'lists', 'match_kind'];
            const orderedEntries = [];
            priorityKeys.forEach(function(key) {
                const found = entries.find(function([k]) { return k === key; });
                if (found) orderedEntries.push(found);
            });
            entries.forEach(function(entry) {
                if (!priorityKeys.includes(entry[0])) {
                    orderedEntries.push(entry);
                }
            });

            const container = document.createElement('div');
            container.className = 'trace-metadata';
            orderedEntries.forEach(function ([key, value]) {
                const row = document.createElement('div');
                row.className = 'trace-meta-row';

                // Highlight pattern and lists rows
                if (key === 'pattern' || key === 'lists') {
                    row.classList.add('trace-meta-highlight');
                }

                const keyEl = document.createElement('div');
                keyEl.className = 'trace-meta-key';
                keyEl.textContent = humanizeLabel(key);
                row.appendChild(keyEl);

                const valueEl = document.createElement('div');
                valueEl.className = 'trace-meta-value';
                let valueText;
                if (typeof value === 'string') {
                    valueText = value;
                } else if (typeof value === 'number' || typeof value === 'boolean') {
                    valueText = String(value);
                } else {
                    try {
                        valueText = JSON.stringify(value);
                    } catch (_) {
                        valueText = String(value);
                    }
                }
                valueEl.textContent = valueText;
                row.appendChild(valueEl);

                container.appendChild(row);
            });
            return container;
        }

        function escapeHtml(value) {
            const div = document.createElement('div');
            div.textContent = value == null ? '' : String(value);
            return div.innerHTML;
        }

        function buildRegexFromPattern(pattern) {
            if (!pattern) return null;
            try {
                // Treat '*' as a wildcard for any characters, escape everything else
                const escaped = pattern
                    .replace(/[-/\\^$+?.()|[\]{}]/g, '\\$&')
                    .replace(/\*/g, '.*');
                return new RegExp(escaped, 'gi');
            } catch (e) {
                try {
                    return new RegExp(pattern, 'gi');
                } catch (_) {
                    return null;
                }
            }
        }

        function createHighlightedDomain(domain, entry) {
            if (!domain) return null;

            const pattern = entry && entry.metadata
                ? (entry.metadata.pattern || entry.metadata.match || entry.metadata.matched_fragment || entry.rule || entry.detail)
                : (entry.rule || entry.detail);
            const regex = buildRegexFromPattern(pattern);
            if (!regex) {
                return document.createTextNode(domain);
            }

            const frag = document.createDocumentFragment();
            let lastIndex = 0;
            let match;
            while ((match = regex.exec(domain)) !== null) {
                if (match.index > lastIndex) {
                    frag.appendChild(document.createTextNode(domain.slice(lastIndex, match.index)));
                }
                const mark = document.createElement('mark');
                mark.className = 'trace-highlight';
                mark.textContent = match[0] || '';
                frag.appendChild(mark);
                lastIndex = regex.lastIndex;
                if (regex.lastIndex === match.index) {
                    // Avoid infinite loops on zero-length matches
                    regex.lastIndex++;
                }
            }

            if (lastIndex < domain.length) {
                frag.appendChild(document.createTextNode(domain.slice(lastIndex)));
            }
            return frag;
        }

        function renderDomainHighlight(domain, entry) {
            if (!domain) return null;

            const wrapper = document.createElement('div');
            wrapper.className = 'trace-domain';

            const label = document.createElement('div');
            label.className = 'trace-meta-key';
            label.textContent = 'Domain';

            const value = document.createElement('div');
            value.className = 'trace-meta-value';
            const highlighted = createHighlightedDomain(domain, entry);
            if (highlighted) {
                value.appendChild(highlighted);
            } else {
                value.textContent = domain;
            }

            wrapper.appendChild(label);
            wrapper.appendChild(value);
            return wrapper;
        }

        function renderTraceRow(entry, domain) {
            const row = document.createElement('div');
            row.className = 'trace-row';

            const header = document.createElement('div');
            header.className = 'trace-row-header';

            const stageWrapper = document.createElement('div');
            const stage = document.createElement('div');
            stage.className = 'trace-stage';
            stage.textContent = formatStageLabel(entry.stage);
            stageWrapper.appendChild(stage);

            if (entry.stage) {
                const stageMeta = document.createElement('div');
                stageMeta.className = 'trace-stage-meta';
                stageMeta.textContent = entry.stage;
                stageWrapper.appendChild(stageMeta);
            }

            header.appendChild(stageWrapper);

            const chips = renderTraceChips(entry);
            if (chips) {
                header.appendChild(chips);
            }

            row.appendChild(header);

            if (entry.detail) {
                const detail = document.createElement('p');
                detail.className = 'trace-detail';
                detail.textContent = entry.detail;
                row.appendChild(detail);
            }

            const metadata = renderTraceMetadata(entry.metadata);
            const domainHighlight = renderDomainHighlight(domain, entry);
            if (metadata) {
                if (domainHighlight) {
                    metadata.prepend(domainHighlight);
                }
                row.appendChild(metadata);
            } else if (domainHighlight) {
                const meta = document.createElement('div');
                meta.className = 'trace-metadata';
                meta.appendChild(domainHighlight);
                row.appendChild(meta);
            }

            return row;
        }

        function openTraceModal(entries, domain, client) {
            traceBody.innerHTML = '';

            const summary = document.createElement('div');
            summary.className = 'trace-summary';

            // Use first entry metadata to power the highlight if available
            const firstEntry = (entries && entries.length) ? entries.find(e => e.metadata && (e.metadata.pattern || e.metadata.match || e.metadata.matched_fragment)) || entries[0] : null;

            const domainBlock = renderDomainHighlight(domain || 'unknown domain', firstEntry || {});
            if (domainBlock) {
                summary.appendChild(domainBlock);
            }

            const clientBlock = document.createElement('div');
            clientBlock.className = 'trace-summary-client';
            const clientLabel = document.createElement('div');
            clientLabel.className = 'trace-meta-key';
            clientLabel.textContent = 'Client';
            const clientVal = document.createElement('div');
            clientVal.className = 'trace-meta-value';
            clientVal.textContent = client || 'unknown client';
            clientBlock.appendChild(clientLabel);
            clientBlock.appendChild(clientVal);
            summary.appendChild(clientBlock);

            traceBody.appendChild(summary);

            if (!entries || !entries.length) {
                const empty = document.createElement('p');
                empty.className = 'text-muted';
                empty.textContent = 'No decision trace recorded.';
                traceBody.appendChild(empty);
            } else {
                entries.forEach(function (entry) {
                    traceBody.appendChild(renderTraceRow(entry, domain));
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
