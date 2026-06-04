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
  assert.match(
    readSource('pages/Setting/Operation/SettingsSidebarModulesAdmin.jsx'),
    /canvas:\s*true/
  );
  assert.match(
    readSource('components/settings/personal/cards/NotificationSettings.jsx'),
    /canvas:\s*true/
  );

  const userSettingsSource = readSource(
    'pages/Setting/Personal/SettingsSidebarModulesUser.jsx'
  );
  assert.match(
    userSettingsSource,
    /canvas:\s*isSidebarModuleAllowed\('chat', 'canvas'\)/
  );
  assert.match(userSettingsSource, /key:\s*'canvas'/);
});

test('classic canvas launcher builds session based New API URL', () => {
  const source = readSource('helpers/canvas.js');

  assert.match(source, /mode['"]?,\s*['"]newapi/);
  assert.match(source, /baseUrl['"]?,\s*`\$\{normalizedOrigin\}\/canvas`/);
  assert.match(source, /group['"]?,\s*group/);
});
