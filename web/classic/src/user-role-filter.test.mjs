import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const root = dirname(fileURLToPath(import.meta.url));
const readSource = (...parts) => readFileSync(resolve(root, ...parts), 'utf8');

test('classic user list exposes all supported role filters', () => {
  const source = readSource('components/table/users/UsersFilters.jsx');

  assert.match(source, /field='searchRole'/);
  assert.match(source, /label: t\('普通用户'\), value: '1'/);
  assert.match(source, /label: t\('管理员'\), value: '10'/);
  assert.match(source, /label: t\('超级管理员'\), value: '100'/);
});

test('classic user role remains in search, pagination and refresh requests', () => {
  const source = readSource('hooks/users/useUsersData.jsx');

  assert.match(source, /searchRole: formValues\.searchRole \|\| ''/);
  assert.match(source, /role=\$\{encodeURIComponent\(searchRole\)\}/);
  assert.match(
    source,
    /searchUsers\(1, size, searchKeyword, searchGroup, searchRole\)/,
  );
  assert.match(
    source,
    /searchUsers\([\s\S]*?pageSize,[\s\S]*?searchKeyword,[\s\S]*?searchGroup,[\s\S]*?searchRole/,
  );
});
