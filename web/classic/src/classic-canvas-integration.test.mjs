import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const root = dirname(fileURLToPath(import.meta.url));

const readSource = (...parts) => readFileSync(resolve(root, ...parts), 'utf8');

test('classic sidebar exposes Infinite Canvas navigation', () => {
  const source = readSource('components/layout/SiderBar.jsx');

  assert.match(source, /canvas:\s*'\/console\/canvas'/);
  assert.match(source, /itemKey:\s*'canvas'/);
  assert.match(source, /t\('无限画布'\)/);
});

test('classic router mounts the canvas launcher page', () => {
  const source = readSource('App.jsx');

  assert.match(source, /import Canvas from '\.\/pages\/Canvas'/);
  assert.match(source, /path='\/console\/canvas'/);
});

test('classic sidebar configuration defaults include canvas', () => {
  assert.match(readSource('hooks/common/useSidebar.js'), /canvas:\s*true/);
  assert.match(readSource('hooks/common/useSidebar.js'), /customItems:\s*\[\]/);
  assert.match(readSource('hooks/common/useSidebar.js'), /canvasOrigin/);
  assert.match(readSource('hooks/common/useSidebar.js'), /canvasIcon/);
  assert.match(
    readSource('pages/Setting/Operation/SettingsSidebarModulesAdmin.jsx'),
    /key:\s*'canvas'/,
  );
  assert.match(
    readSource('pages/Setting/Operation/SettingsSidebarModulesAdmin.jsx'),
    /自定义侧边栏/,
  );
  assert.match(
    readSource('pages/Setting/Operation/SettingsSidebarModulesAdmin.jsx'),
    /CUSTOM_NAV_ICON_OPTIONS/,
  );
  assert.match(
    readSource('components/settings/personal/cards/NotificationSettings.jsx'),
    /canvas:\s*true/,
  );

  const userSettingsSource = readSource(
    'pages/Setting/Personal/SettingsSidebarModulesUser.jsx',
  );
  assert.match(
    userSettingsSource,
    /canvas:\s*isSidebarModuleAllowed\('chat', 'canvas'\)/,
  );
  assert.match(userSettingsSource, /key:\s*'canvas'/);
});

test('classic canvas launcher builds session based New API URL', () => {
  const source = readSource('helpers/canvas.js');

  assert.match(source, /normalizeCanvasOrigin/);
  assert.match(source, /getCanvasSettingsFromSidebarModules/);
  assert.match(source, /mode['"]?,\s*['"]newapi/);
  assert.match(source, /baseUrl['"]?,\s*`\$\{normalizedOrigin\}\/canvas`/);
  assert.match(source, /group['"]?,\s*group/);
  assert.match(source, /textGroup/);
  assert.match(source, /imageGroup/);
  assert.match(source, /audioGroup/);
  assert.match(source, /videoGroup/);
  assert.match(source, /searchParams\.set\('textGroup'/);
});

test('classic canvas launcher exposes optional capability groups', () => {
  const source = readSource('pages/Canvas/index.jsx');

  assert.match(source, /getCanvasSettingsFromSidebarModules/);
  assert.match(source, /canvasSettings\.canvasOrigin/);
  assert.match(source, /defaultGroup/);
  assert.match(source, /textGroup/);
  assert.match(source, /imageGroup/);
  assert.match(source, /audioGroup/);
  assert.match(source, /videoGroup/);
  assert.match(source, /文本分组/);
  assert.match(source, /生图分组/);
  assert.match(source, /音频分组/);
  assert.match(source, /视频分组/);
  assert.match(source, /showClear/);
});

test('classic group selector displays group names instead of descriptions', () => {
  const source = readSource('helpers/api.js');

  assert.match(source, /label:\s*group/);
  assert.match(source, /value:\s*group/);
  assert.doesNotMatch(source, /label:\s*\n\s*info\.desc/);
});
