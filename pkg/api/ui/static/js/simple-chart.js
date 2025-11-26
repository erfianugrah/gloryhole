(function () {
  const COLOR_VARS = {
    axis: '--chart-axis',
    grid: '--chart-grid',
    fill: '--chart-fill',
    text: '--text'
  };

  function readVar(canvas, name, fallback) {
    const styles = canvas ? getComputedStyle(canvas) : null;
    if (!styles) {
      return fallback;
    }
    const value = styles.getPropertyValue(name);
    return value && value.trim().length ? value.trim() : fallback;
  }

  function resolveColor(canvas, value, fallback) {
    if (Array.isArray(value)) {
      return value[0] || fallback;
    }
    if (typeof value === 'string' && value.trim().length) {
      return value;
    }
    return fallback || readVar(canvas, COLOR_VARS.axis, '#3b82f6');
  }

  class SimpleChart {
    constructor(canvas, config) {
      this.canvas = canvas instanceof HTMLCanvasElement ? canvas : canvas?.canvas;
      if (!this.canvas) {
        throw new Error('SimpleChart requires a canvas element');
      }
      this.ctx = this.canvas.getContext('2d');
      this.config = config || {};
      this.type = (this.config.type || 'line').toLowerCase();
      this.data = this.config.data || { labels: [], datasets: [] };
      this.pixelRatio = window.devicePixelRatio || 1;
      this.frame = null;
      this.colors = this.readColors();

      this.handleResize = () => this.scheduleRender();
      this.handleThemeChange = () => {
        this.colors = this.readColors();
        this.scheduleRender();
      };

      if (window.ResizeObserver) {
        this.resizeObserver = new ResizeObserver(() => this.scheduleRender());
        this.resizeObserver.observe(this.canvas);
      } else {
        window.addEventListener('resize', this.handleResize);
      }
      window.addEventListener('gh-theme-change', this.handleThemeChange);

      this.scheduleRender();
    }

    readColors() {
      return {
        axis: readVar(this.canvas, COLOR_VARS.axis, 'rgba(148,163,184,0.7)'),
        grid: readVar(this.canvas, COLOR_VARS.grid, 'rgba(148,163,184,0.25)'),
        fill: readVar(this.canvas, COLOR_VARS.fill, 'rgba(59,130,246,0.12)'),
        text: readVar(this.canvas, COLOR_VARS.text, '#111827')
      };
    }

    update() {
      this.scheduleRender();
    }

    destroy() {
      if (this.resizeObserver) {
        this.resizeObserver.disconnect();
      } else {
        window.removeEventListener('resize', this.handleResize);
      }
      window.removeEventListener('gh-theme-change', this.handleThemeChange);
      if (this.frame) {
        cancelAnimationFrame(this.frame);
        this.frame = null;
      }
    }

    scheduleRender() {
      if (this.frame) {
        return;
      }
      this.frame = requestAnimationFrame(() => {
        this.frame = null;
        this.render();
      });
    }

    render() {
      if (!this.ctx) {
        return;
      }
      const rect = this.canvas.getBoundingClientRect();
      const width = Math.max(rect.width || this.canvas.width || 100, 50);
      const height = Math.max(rect.height || this.canvas.height || 100, 50);
      const dpr = window.devicePixelRatio || this.pixelRatio || 1;
      this.canvas.width = width * dpr;
      this.canvas.height = height * dpr;
      this.ctx.save();
      this.ctx.scale(dpr, dpr);
      this.ctx.clearRect(0, 0, width, height);

      switch (this.type) {
        case 'bar':
          this.drawBar(width, height);
          break;
        case 'doughnut':
          this.drawDoughnut(width, height);
          break;
        default:
          this.drawLine(width, height);
      }
      this.ctx.restore();
    }

    drawAxes(width, height, maxValue) {
      const ctx = this.ctx;
      const left = 40;
      const top = 20;
      const bottom = height - 24;
      ctx.strokeStyle = this.colors.axis;
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(left, top);
      ctx.lineTo(left, bottom);
      ctx.lineTo(width - 10, bottom);
      ctx.stroke();

      const steps = 4;
      ctx.fillStyle = this.colors.axis;
      ctx.font = '11px system-ui, sans-serif';
      ctx.textBaseline = 'middle';
      ctx.textAlign = 'right';

      for (let i = 0; i <= steps; i += 1) {
        const value = Math.round((maxValue / steps) * i);
        const y = bottom - ((bottom - top) / steps) * i;
        ctx.strokeStyle = this.colors.grid;
        ctx.beginPath();
        ctx.moveTo(left, y);
        ctx.lineTo(width - 10, y);
        ctx.stroke();
        ctx.fillStyle = this.colors.axis;
        ctx.fillText(value.toString(), left - 6, y);
      }

      return { left, right: width - 10, top, bottom };
    }

    drawLine(width, height) {
      const datasets = Array.isArray(this.data.datasets) ? this.data.datasets : [];
      const labels = Array.isArray(this.data.labels) ? this.data.labels : [];
      const values = datasets.flatMap((ds) => ds.data || []);
      const maxValue = Math.max(...values, 1);
      const area = this.drawAxes(width, height, maxValue);
      const span = Math.max(labels.length - 1, 1);
      const xStep = span === 0 ? 0 : (area.right - area.left) / span;

      datasets.forEach((dataset) => {
        const data = dataset.data || [];
        if (!data.length) {
          return;
        }
        const stroke = resolveColor(this.canvas, dataset.borderColor, '#3b82f6');
        const fill = resolveColor(this.canvas, dataset.backgroundColor, this.colors.fill);
        const points = data.map((value, index) => {
          const clamped = Math.max(0, Number(value) || 0);
          const ratio = Math.min(1, clamped / maxValue);
          const x = area.left + index * xStep;
          const y = area.bottom - ratio * (area.bottom - area.top);
          return { x, y };
        });

        this.ctx.lineWidth = 2;
        this.ctx.strokeStyle = stroke;
        this.ctx.lineJoin = 'round';
        this.ctx.lineCap = 'round';
        this.ctx.beginPath();
        points.forEach((point, index) => {
          if (index === 0) {
            this.ctx.moveTo(point.x, point.y);
          } else {
            this.ctx.lineTo(point.x, point.y);
          }
        });
        this.ctx.stroke();

        if (dataset.fill) {
          const gradient = this.ctx.createLinearGradient(0, area.top, 0, area.bottom);
          gradient.addColorStop(0, fill);
          gradient.addColorStop(1, 'rgba(0,0,0,0)');
          this.ctx.beginPath();
          this.ctx.moveTo(points[0].x, area.bottom);
          points.forEach((pt) => this.ctx.lineTo(pt.x, pt.y));
          this.ctx.lineTo(points[points.length - 1].x, area.bottom);
          this.ctx.closePath();
          this.ctx.fillStyle = gradient;
          this.ctx.fill();
        }
      });
    }

    drawBar(width, height) {
      const dataset = (this.data.datasets && this.data.datasets[0]) || { data: [] };
      const values = dataset.data || [];
      const labels = this.data.labels || [];
      const maxValue = Math.max(...values, 1);
      const area = this.drawAxes(width, height, maxValue);
      const count = Math.max(values.length, 1);
      const totalWidth = area.right - area.left;
      const spacing = 0.25;
      const barWidth = Math.max(totalWidth / (count + (count - 1) * spacing), 10);
      const gap = barWidth * spacing;
      const color = resolveColor(this.canvas, dataset.backgroundColor, '#3b82f6');

      values.forEach((raw, index) => {
        const value = Math.max(0, Number(raw) || 0);
        const ratio = Math.min(1, value / maxValue);
        const x = area.left + index * (barWidth + gap);
        const barHeight = ratio * (area.bottom - area.top);
        const y = area.bottom - barHeight;
        this.ctx.fillStyle = color;
        const heightValue = Math.max(barHeight, 2);
        if (typeof this.ctx.roundRect === 'function') {
          this.ctx.beginPath();
          this.ctx.roundRect(x, y, barWidth, heightValue, 4);
          this.ctx.fill();
        } else {
          this.ctx.fillRect(x, y, barWidth, heightValue);
        }

        if (labels[index]) {
          this.ctx.save();
          this.ctx.fillStyle = this.colors.text;
          this.ctx.font = '10px system-ui, sans-serif';
          this.ctx.translate(x + barWidth / 2, area.bottom + 14);
          this.ctx.rotate(-Math.PI / 10);
          this.ctx.textAlign = 'center';
          this.ctx.fillText(String(labels[index]), 0, 0);
          this.ctx.restore();
        }
      });
    }

    drawDoughnut(width, height) {
      const dataset = (this.data.datasets && this.data.datasets[0]) || { data: [] };
      const values = dataset.data || [];
      const labels = this.data.labels || [];
      const total = values.reduce((sum, value) => sum + Math.max(0, Number(value) || 0), 0) || 1;
      const radius = Math.min(width, height) / 2 - 12;
      const centerX = width / 2;
      const centerY = height / 2;
      const colors = Array.isArray(dataset.backgroundColor)
        ? dataset.backgroundColor
        : labels.map((_, idx) => resolveColor(this.canvas, dataset.backgroundColor, '#3b82f6'));

      let startAngle = -Math.PI / 2;
      values.forEach((rawValue, index) => {
        const value = Math.max(0, Number(rawValue) || 0);
        const slice = (value / total) * Math.PI * 2;
        const endAngle = startAngle + slice;
        this.ctx.beginPath();
        this.ctx.moveTo(centerX, centerY);
        this.ctx.arc(centerX, centerY, radius, startAngle, endAngle);
        this.ctx.closePath();
        this.ctx.fillStyle = colors[index % colors.length];
        this.ctx.fill();
        startAngle = endAngle;
      });

      this.ctx.beginPath();
      this.ctx.fillStyle = readVar(this.canvas, '--surface', '#ffffff');
      this.ctx.arc(centerX, centerY, radius * 0.6, 0, Math.PI * 2);
      this.ctx.fill();

      this.ctx.fillStyle = this.colors.text;
      this.ctx.font = '12px system-ui, sans-serif';
      this.ctx.textAlign = 'center';
      this.ctx.fillText(`${total} total`, centerX, centerY + 4);
    }
  }

  if (!window.Chart) {
    window.Chart = SimpleChart;
  } else {
    window.SimpleChart = SimpleChart;
  }
})();
