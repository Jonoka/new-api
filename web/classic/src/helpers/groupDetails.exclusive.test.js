/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { describe, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import {
  createUserGroupOptions,
  includeSelectedGroupOptions,
  normalizeGroupDetail,
} from './groupDetails.js';

const root = dirname(fileURLToPath(import.meta.url));

describe('独立令牌分组', () => {
  test('分组配置和用户选项保留独立属性', () => {
    assert.equal(
      normalizeGroupDetail({ code: 'hack', exclusive: true }).exclusive,
      true,
    );
    assert.equal(
      createUserGroupOptions({
        hack: { code: 'hack', name: 'Hack', exclusive: true },
      })[0]?.exclusive,
      true,
    );
  });

  test('Classic 令牌多选器禁止独立分组与其他分组混选', () => {
    const source = readFileSync(
      resolve(root, '../components/table/tokens/modals/EditTokenModal.jsx'),
      'utf8',
    );
    assert.match(source, /isExclusiveSelected/);
    assert.match(source, /g\.exclusive === true && selectedGroups\.length > 0/);
    assert.match(source, /onChange\(\[value\]\)/);
  });

  test('历史独立分组不在当前可用列表时仍保留独立属性', () => {
    const options = includeSelectedGroupOptions(
      [],
      ['hack'],
      [{ id: 7, code: 'hack', name: 'Hack', exclusive: true }],
    );

    assert.equal(options.length, 1);
    assert.equal(options[0].value, 'hack');
    assert.equal(options[0].exclusive, true);
  });
});
