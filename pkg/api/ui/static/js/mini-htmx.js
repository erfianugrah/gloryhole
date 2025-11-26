(function () {
  const stateMap = new WeakMap();

  function getState(el) {
    let state = stateMap.get(el);
    if (!state) {
      state = { intervals: [], initialized: false };
      stateMap.set(el, state);
    }
    return state;
  }

  function resolveEventSource(source, element) {
    if (!source || source === 'this') {
      return element;
    }
    const name = source.trim().toLowerCase();
    if (name === 'document') {
      return document;
    }
    if (name === 'body') {
      return document.body;
    }
    if (name === 'window') {
      return window;
    }
    return document.querySelector(source) || element;
  }

  function parseIntervalMs(token) {
    const match = token.match(/every\s+(\d+(?:\.\d+)?)(ms|s|m)?/i);
    if (!match) {
      return null;
    }
    const value = parseFloat(match[1]);
    const unit = (match[2] || 's').toLowerCase();
    if (Number.isNaN(value)) {
      return null;
    }
    switch (unit) {
      case 'ms':
        return value;
      case 'm':
        return value * 60000;
      default:
        return value * 1000;
    }
  }

  function parseTriggers(attributeValue) {
    if (!attributeValue) {
      return [{ type: 'load' }];
    }
    const triggers = [];
    attributeValue
      .split(',')
      .map((token) => token.trim())
      .filter(Boolean)
      .forEach((token) => {
        if (/^every\s+/i.test(token)) {
          const ms = parseIntervalMs(token);
          if (ms) {
            triggers.push({ type: 'interval', ms });
          }
          return;
        }
        if (token === 'load') {
          triggers.push({ type: 'load' });
          return;
        }
        const eventMatch = token.match(/([^\s]+)(?:\s+from:(.+))?/i);
        if (eventMatch) {
          triggers.push({
            type: 'event',
            name: eventMatch[1],
            source: eventMatch[2] ? eventMatch[2].trim() : null,
          });
        }
      });
    return triggers.length ? triggers : [{ type: 'load' }];
  }

  function applySwap(sourceEl, html, mode) {
    const targetSelector = sourceEl.getAttribute('hx-target');
    const target = targetSelector ? document.querySelector(targetSelector) : sourceEl;
    if (!target) {
      return;
    }
    const swapMode = (mode || sourceEl.getAttribute('hx-swap') || 'innerHTML').toLowerCase();
    if (swapMode === 'outerhtml') {
      const template = document.createElement('template');
      template.innerHTML = html.trim();
      const replacement = template.content.firstElementChild;
      if (replacement) {
        target.replaceWith(replacement);
        initializeHx(replacement);
      } else {
        target.outerHTML = html;
      }
      return;
    }
    target.innerHTML = html;
    initializeHx(target);
  }

  async function performRequest(el, options) {
    const url = el.getAttribute('hx-get');
    if (!url) {
      return;
    }
    const fetchOptions = {
      method: (options && options.method) || 'GET',
      headers: {
        'HX-Request': 'true',
      },
    };
    if (options && options.body) {
      fetchOptions.body = options.body;
      fetchOptions.headers['Content-Type'] = options.contentType;
    }
    try {
      const response = await fetch(url, fetchOptions);
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }
      const payload = await response.text();
      applySwap(el, payload);
    } catch (error) {
      console.error('mini-htmx request failed', error);
    }
  }

  async function submitForm(form) {
    const url = form.getAttribute('hx-put');
    if (!url) {
      return;
    }
    const targetSelector = form.getAttribute('hx-target');
    const target = targetSelector ? document.querySelector(targetSelector) : form;
    if (!target) {
      return;
    }
    const swapMode = form.getAttribute('hx-swap') || 'innerHTML';
    const formData = new FormData(form);
    const body = new URLSearchParams();
    formData.forEach((value, key) => {
      body.append(key, value);
    });
    try {
      const response = await fetch(url, {
        method: 'PUT',
        headers: {
          'HX-Request': 'true',
          'Content-Type': 'application/x-www-form-urlencoded; charset=UTF-8',
        },
        body: body.toString(),
      });
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }
      const payload = await response.text();
      applySwap(target, payload, swapMode);
    } catch (error) {
      console.error('mini-htmx form submission failed', error);
    }
  }

  function setupHxGet(el) {
    const state = getState(el);
    if (state.getInitialized) {
      return;
    }
    state.getInitialized = true;
    const triggers = parseTriggers(el.getAttribute('hx-trigger'));
    let initialScheduled = false;

    const scheduleInitial = () => {
      if (initialScheduled) {
        return;
      }
      initialScheduled = true;
      if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', () => performRequest(el), { once: true });
      } else {
        queueMicrotask(() => performRequest(el));
      }
    };

    triggers.forEach((trigger) => {
      if (trigger.type === 'load') {
        el.addEventListener('load', () => performRequest(el));
        scheduleInitial();
        return;
      }
      if (trigger.type === 'interval') {
        const id = setInterval(() => performRequest(el), trigger.ms);
        state.intervals.push(id);
        scheduleInitial();
        return;
      }
      if (trigger.type === 'event') {
        const source = resolveEventSource(trigger.source || 'this', el);
        if (source) {
          source.addEventListener(trigger.name, () => performRequest(el));
        }
      }
    });

    if (!initialScheduled && triggers.length === 0) {
      scheduleInitial();
    }
  }

  function setupHxPut(form) {
    if (getState(form).putInitialized) {
      return;
    }
    getState(form).putInitialized = true;
    form.addEventListener('submit', (event) => {
      event.preventDefault();
      submitForm(form);
    });
  }

  function processElement(el) {
    if (!(el instanceof Element)) {
      return;
    }
    if (el.hasAttribute('hx-get')) {
      setupHxGet(el);
    }
    if (el.tagName === 'FORM' && el.hasAttribute('hx-put')) {
      setupHxPut(el);
    }
  }

  function initializeHx(root) {
    if (!root) {
      return;
    }
    if (root instanceof Element) {
      processElement(root);
      root.querySelectorAll('[hx-get], form[hx-put]').forEach((el) => processElement(el));
    } else if (root.querySelectorAll) {
      root.querySelectorAll('[hx-get], form[hx-put]').forEach((el) => processElement(el));
    }
  }

  if (!window.htmx) {
    window.htmx = {};
  }

  window.htmx.trigger = function (target, eventName, detail) {
    if (!target) {
      return;
    }
    const event = new CustomEvent(eventName, {
      bubbles: true,
      detail: detail || null,
    });
    target.dispatchEvent(event);
  };

  document.addEventListener('DOMContentLoaded', () => {
    initializeHx(document.body || document);
  });
})();
