#!/usr/bin/env node

const fs = require('fs');
const path = require('path');

const VENDOR_DIR = 'pkg/api/ui/static/js/vendor';
const FONTS_DIR = 'pkg/api/ui/static/fonts';

// Create directories if they don't exist
if (!fs.existsSync(VENDOR_DIR)) {
  fs.mkdirSync(VENDOR_DIR, { recursive: true });
}
if (!fs.existsSync(FONTS_DIR)) {
  fs.mkdirSync(FONTS_DIR, { recursive: true });
}

// Copy JS vendor files
const vendorFiles = [
  {
    from: 'node_modules/htmx.org/dist/htmx.min.js',
    to: path.join(VENDOR_DIR, 'htmx.min.js')
  },
  {
    from: 'node_modules/idiomorph/dist/idiomorph-ext.min.js',
    to: path.join(VENDOR_DIR, 'idiomorph-ext.min.js')
  },
  {
    from: 'node_modules/chart.js/dist/chart.umd.min.js',
    to: path.join(VENDOR_DIR, 'chart.umd.min.js')
  }
];

console.log('📦 Copying vendor JS files...');
vendorFiles.forEach(({ from, to }) => {
  if (fs.existsSync(from)) {
    fs.copyFileSync(from, to);
    console.log(`  ✓ ${path.basename(to)}`);
  } else {
    console.error(`  ✗ Not found: ${from}`);
    process.exit(1);
  }
});

// Copy font files
console.log('\n🔤 Copying font files...');

// JetBrains Mono
const jetbrainsMonoFiles = [
  'files/jetbrains-mono-latin-400-normal.woff2',
  'files/jetbrains-mono-latin-500-normal.woff2',
  'files/jetbrains-mono-latin-600-normal.woff2',
  'files/jetbrains-mono-latin-700-normal.woff2'
];

jetbrainsMonoFiles.forEach(file => {
  const from = path.join('node_modules/@fontsource/jetbrains-mono', file);
  const to = path.join(FONTS_DIR, path.basename(file));
  if (fs.existsSync(from)) {
    fs.copyFileSync(from, to);
    console.log(`  ✓ ${path.basename(to)}`);
  } else {
    console.warn(`  ⚠ Optional file not found: ${from}`);
  }
});

// Space Grotesk
const spaceGroteskFiles = [
  'files/space-grotesk-latin-400-normal.woff2',
  'files/space-grotesk-latin-500-normal.woff2',
  'files/space-grotesk-latin-600-normal.woff2',
  'files/space-grotesk-latin-700-normal.woff2'
];

spaceGroteskFiles.forEach(file => {
  const from = path.join('node_modules/@fontsource/space-grotesk', file);
  const to = path.join(FONTS_DIR, path.basename(file));
  if (fs.existsSync(from)) {
    fs.copyFileSync(from, to);
    console.log(`  ✓ ${path.basename(to)}`);
  } else {
    console.warn(`  ⚠ Optional file not found: ${from}`);
  }
});

// Create font CSS
const fontCSS = `/* JetBrains Mono */
@font-face {
  font-family: 'JetBrains Mono';
  font-style: normal;
  font-weight: 400;
  font-display: swap;
  src: url('jetbrains-mono-latin-400-normal.woff2') format('woff2');
}

@font-face {
  font-family: 'JetBrains Mono';
  font-style: normal;
  font-weight: 500;
  font-display: swap;
  src: url('jetbrains-mono-latin-500-normal.woff2') format('woff2');
}

@font-face {
  font-family: 'JetBrains Mono';
  font-style: normal;
  font-weight: 600;
  font-display: swap;
  src: url('jetbrains-mono-latin-600-normal.woff2') format('woff2');
}

@font-face {
  font-family: 'JetBrains Mono';
  font-style: normal;
  font-weight: 700;
  font-display: swap;
  src: url('jetbrains-mono-latin-700-normal.woff2') format('woff2');
}

/* Space Grotesk */
@font-face {
  font-family: 'Space Grotesk';
  font-style: normal;
  font-weight: 400;
  font-display: swap;
  src: url('space-grotesk-latin-400-normal.woff2') format('woff2');
}

@font-face {
  font-family: 'Space Grotesk';
  font-style: normal;
  font-weight: 500;
  font-display: swap;
  src: url('space-grotesk-latin-500-normal.woff2') format('woff2');
}

@font-face {
  font-family: 'Space Grotesk';
  font-style: normal;
  font-weight: 600;
  font-display: swap;
  src: url('space-grotesk-latin-600-normal.woff2') format('woff2');
}

@font-face {
  font-family: 'Space Grotesk';
  font-style: normal;
  font-weight: 700;
  font-display: swap;
  src: url('space-grotesk-latin-700-normal.woff2') format('woff2');
}
`;

fs.writeFileSync(path.join(FONTS_DIR, 'fonts.css'), fontCSS);
console.log('  ✓ fonts.css');

console.log('\n✨ Vendor files copied successfully!\n');
