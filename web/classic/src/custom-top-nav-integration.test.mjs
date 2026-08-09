import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const readSource = (path) =>
  readFileSync(new URL(path, import.meta.url), 'utf8');

test('custom menu editor exposes the header placement', () => {
  const source = readSource('./helpers/customNav.jsx');

  assert.match(source, /value:\s*'header',\s*label:\s*'顶栏区域'/);
  assert.match(source, /item\.section === 'header'/);
});

test('classic header merges sidebar-managed header items into navigation', () => {
  const headerSource = readSource('./components/layout/headerbar/index.jsx');
  const navigationSource = readSource('./hooks/common/useNavigation.js');

  assert.match(headerSource, /sidebarNavModules/);
  assert.match(
    navigationSource,
    /parseTopNavCustomItems\(\s*modules\.customItems,\s*sidebarNavModules\?\.customItems/,
  );
  assert.match(navigationSource, /requiresAuth:\s*item\.requireAuth/);
});
