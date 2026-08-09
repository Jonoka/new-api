import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { test } from 'node:test'
import { fileURLToPath } from 'node:url'

const root = dirname(fileURLToPath(import.meta.url))
const readSource = (file: string) => readFileSync(resolve(root, file), 'utf8')

test('分组和高级配置通过单次原子请求保存且不使用 staging', () => {
  const formSource = readSource('group-ratio-form.tsx')

  assert.equal([...formSource.matchAll(/saveGroupDetails\(\{/g)].length, 1)
  assert.match(formSource, /option_updates:\s*optionUpdates/)
  assert.match(formSource, /auto_group:\s*autoGroup/)
  assert.match(formSource, /queryKey:\s*\['groups'\]/)
  assert.doesNotMatch(formSource, /prepareGroupNamesForInitialSave/)
  assert.doesNotMatch(formSource, /__group_prepare_/)
})

test('虚拟 auto 行独立保存且不进入实体 groups 数组', () => {
  const formSource = readSource('group-ratio-form.tsx')
  const editorSource = readSource('group-ratio-visual-editor.tsx')

  assert.match(formSource, /auto_group:\s*autoGroup/)
  assert.match(editorSource, /Auto \(Circuit Breaker\)/)
  assert.match(editorSource, /autoGroup\.user_selectable/)
  assert.match(editorSource, /onAutoGroupChange/)
  assert.doesNotMatch(
    formSource.match(/function createGroupDetailsPayload[\s\S]*?\n}/)?.[0] ??
      '',
    /autoGroup/
  )
})

test('高级 Option 更新排除后端投影字段', () => {
  const cardSource = readSource('ratio-settings-card.tsx')
  const saveBlock = cardSource.match(
    /const saveGroupRatios[\s\S]*?const handleResetRatios/
  )?.[0]

  assert.ok(saveBlock)
  assert.match(saveBlock, /TopupGroupRatio:/)
  assert.match(saveBlock, /GroupGroupRatio:/)
  assert.match(saveBlock, /DefaultUseAutoGroup:/)
  assert.match(saveBlock, /GroupSpecialUsableGroup:/)
  assert.doesNotMatch(saveBlock, /\n\s*GroupRatio:/)
  assert.doesNotMatch(saveBlock, /\n\s*UserUsableGroups:/)
  assert.doesNotMatch(saveBlock, /\n\s*AutoGroups:/)
  assert.doesNotMatch(saveBlock, /mutateAsync/)
})
