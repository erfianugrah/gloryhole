(function () {
  function resolveColor(value, fallback) {
    if (Array.isArray(value)) {
      return value[0] || fallback;
    }
    return value || fallback;
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
      this._resizeHandler = () => this.render();
      window.addEventListener('resize', this._resizeHandler);
      this.render();
    }

    update() {
      this.render();
    }

    destroy() {
      window.removeEventListener('resize', this._resizeHandler);
    }

    render() {
      if (!this.ctx) {
        return;
      }
      const rect = this.canvas.getBoundingClientRect();
      const width = Math.max(rect.width, 50);
      const height = Math.max(rect.height, 50);
      const dpr = window.devicePixelRatio || 1;
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
      ctx.strokeStyle = 'rgba(148, 163, 184, 0.6)';
      ctx.lineWidth = 1;
      const left = 32;
      const bottom = height - 20;
      ctx.beginPath();
      ctx.moveTo(left, 10);
      ctx.lineTo(left, bottom);
      ctx.lineTo(width - 10, bottom);
      ctx.stroke();

      const steps = 4;
      ctx.fillStyle = 'rgba(75,85,99,0.7)';
      ctx.font = '10px sans-serif';
      for (let i = 0; i <= steps; i += 1) {
        const value = Math.round((maxValue / steps) * i);
        const y = bottom - ((bottom - 20) / steps) * i;
        ctx.fillText(value.toString(), 4, y + 4);
        ctx.beginPath();
        ctx.moveTo(left - 3, y);
        ctx.lineTo(left, y);
        ctx.stroke();
      }
    }

    drawLine(width, height) {
      const datasets = Array.isArray(this.data.datasets) ? this.data.datasets : [];
      const labels = Array.isArray(this.data.labels) ? this.data.labels : [];
      const values = datasets.flatMap((ds) => ds.data || []);
      const maxValue = Math.max(...values, 1);
      const left = 40;
      const bottom = height - 20;
      const top = 20;
      const right = width - 10;
      const span = Math.max(labels.length - 1, 1);
      const xStep = (right - left) / span;

      this.drawAxes(width, height, maxValue);

      datasets.forEach((dataset) => {
        const data = dataset.data || [];
        if (!data.length) {
          return;
        }
        const color = resolveColor(dataset.borderColor, '#3b82f6');
        const points = [];
        data.forEach((value, index) => {
          const x = left + index * xStep;
          const clamped = Math.max(0, Number(value) || 0);
          const yRange = bottom - top;
          const y = bottom - Math.min(1, clamped / maxValue) * yRange;
          points.push({ x, y });
        });
        if (!points.length) {
          return;
        }
        this.ctx.beginPath();
        points.forEach((point, index) => {
          const { x, y } = point;
          if (index === 0) {
            this.ctx.moveTo(x, y);
          } else {
            this.ctx.lineTo(x, y);
          }
        });
        this.ctx.strokeStyle = color;
        this.ctx.lineWidth = 2;
        this.ctx.stroke();

        if (dataset.fill) {
          const gradient = this.ctx.createLinearGradient(0, top, 0, bottom);
          const fillColor = resolveColor(dataset.backgroundColor, 'rgba(59,130,246,0.15)');
          gradient.addColorStop(0, fillColor);
          gradient.addColorStop(1, 'rgba(255,255,255,0)');
          this.ctx.lineTo(points[points.length - 1].x, bottom);
          this.ctx.lineTo(points[0].x, bottom);
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
      const count = Math.max(values.length, 1);
      const barAreaWidth = width - 40;
      const barWidth = Math.max(barAreaWidth / (count * 1.6), 8);
      const maxValue = Math.max(...values, 1);
      const bottom = height - 20;
      const top = 20;
      const left = 30;

      this.drawAxes(width, height, maxValue);

      values.forEach((rawValue, index) => {
        const value = Math.max(0, Number(rawValue) || 0);
        const x = left + index * (barWidth * 1.6);
        const yHeight = ((bottom - top) * value) / maxValue;
        const y = bottom - yHeight;
        const color = resolveColor(dataset.backgroundColor, '#3b82f6');
        this.ctx.fillStyle = color;
        this.ctx.fillRect(x, y, barWidth, Math.max(yHeight, 2));
        if (labels[index]) {
          this.ctx.fillStyle = 'rgba(75,85,99,0.9)';
          this.ctx.font = '10px sans-serif';
          this.ctx.save();
          this.ctx.translate(x + barWidth / 2, bottom + 12);
          this.ctx.rotate(-Math.PI / 8);
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
      const radius = Math.min(width, height) / 2 - 10;
      const centerX = width / 2;
      const centerY = height / 2;
      const colors = Array.isArray(dataset.backgroundColor)
        ? dataset.backgroundColor
        : labels.map(() => resolveColor(dataset.backgroundColor, '#3b82f6'));

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
      this.ctx.fillStyle = '#fff';
      this.ctx.moveTo(centerX, centerY);
      this.ctx.arc(centerX, centerY, radius * 0.6, 0, Math.PI * 2);
      this.ctx.fill();

      this.ctx.fillStyle = 'rgba(75,85,99,0.9)';
      this.ctx.font = '12px sans-serif';
      this.ctx.textAlign = 'center';
      this.ctx.fillText(`${total} total`, centerX, centerY + 4);
    }
  }

  window.Chart = SimpleChart;
})();
