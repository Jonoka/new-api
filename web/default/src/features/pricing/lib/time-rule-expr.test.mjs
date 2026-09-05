import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import * as classicRules from '../../../../../classic/src/pages/Setting/Ratio/components/requestRuleExpr.js'
import * as defaultRules from './billing-expr.ts'

for (const [theme, rules] of [
  ['default', defaultRules],
  ['classic', classicRules],
]) {
  const condition = (overrides = {}) => ({
    source: 'time',
    timeFunc: 'hour',
    timezone: 'UTC',
    mode: 'range',
    value: '',
    rangeStart: '9',
    rangeEnd: '12',
    ...overrides,
  })
  const group = (overrides = {}) => ({
    conditions: [condition(overrides)],
    multiplier: '2',
  })

  describe(`${theme} time pricing rules`, () => {
    test('evaluates every hour for within-day, overnight and equal ranges', () => {
      for (const [start, end] of [
        [9, 12],
        [21, 6],
        [9, 9],
      ]) {
        const expression = rules.buildRequestRuleExpr([
          group({ rangeStart: String(start), rangeEnd: String(end) }),
        ])
        const evaluate = new Function('hour', `return ${expression}`)
        for (let hour = 0; hour < 24; hour += 1) {
          const applies =
            start < end
              ? hour >= start && hour < end
              : start > end && (hour >= start || hour < end)
          assert.equal(
            evaluate(() => hour),
            applies ? 2 : 1,
            `${start}..${end} at ${hour}`
          )
        }
      }
    })

    test('compound within-day and overnight ranges round-trip with parameter guards', () => {
      const groups = [
        {
          conditions: [
            {
              source: 'param',
              path: 'service_tier',
              mode: 'eq',
              value: 'fast',
            },
            condition(),
            { source: 'param', path: 'n', mode: 'gte', value: '2' },
          ],
          multiplier: '3',
        },
        group({ rangeStart: '21', rangeEnd: '6' }),
      ]
      const expression = rules.buildRequestRuleExpr(groups)
      const parsed = rules.tryParseRequestRuleExpr(expression)
      assert.ok(parsed)
      assert.equal(parsed[0].conditions.length, 3)
      assert.equal(parsed[0].conditions[1].mode, 'range')
      assert.equal(rules.buildRequestRuleExpr(parsed), expression)
    })

    test('legacy tautologies and mismatched wrapped ranges remain raw', () => {
      for (const expression of [
        '(hour("UTC") >= 9 || hour("UTC") < 12 ? 2 : 1)',
        '(hour("UTC") >= 9 || hour("UTC") < 9 ? 2 : 1)',
        '((hour("UTC") >= 21 && hour("UTC") < 6) ? 2 : 1)',
      ]) {
        assert.equal(rules.tryParseRequestRuleExpr(expression), null)
        const combined = rules.combineBillingExpr(
          'tier("base", p * 2)',
          expression
        )
        const split = rules.splitBillingExprAndRequestRules(combined)
        assert.equal(
          rules.combineBillingExpr(split.billingExpr, split.requestRuleExpr),
          combined
        )
      }
    })

    test('rejects invalid domains, fractions and incomplete rules without dropping a condition', () => {
      for (const overrides of [
        { rangeStart: '-1' },
        { rangeEnd: '24' },
        { rangeStart: '9.5' },
        { rangeStart: '' },
        { rangeEnd: 'Infinity' },
        { mode: 'gte', value: '24' },
        { timeFunc: 'month', mode: 'gte', value: '0' },
      ]) {
        const invalid = group(overrides)
        invalid.conditions.push({
          source: 'header',
          path: 'x-test',
          mode: 'exists',
          value: '',
        })
        assert.equal(rules.areRequestRuleGroupsValid([invalid]), false)
        assert.equal(rules.buildRequestRuleExpr([group(), invalid]), '')
      }
      assert.equal(rules.areRequestRuleGroupsValid([]), true)
      assert.equal(rules.buildRequestRuleExpr([]), '')
    })

    test('enforces each time-function domain while preserving valid scalar values', () => {
      for (const [timeFunc, min, max] of [
        ['hour', 0, 23],
        ['minute', 0, 59],
        ['weekday', 0, 6],
        ['month', 1, 12],
        ['day', 1, 31],
      ]) {
        for (const value of [min, max]) {
          const groups = [
            group({ timeFunc, mode: 'gte', value: String(value) }),
          ]
          assert.equal(rules.areRequestRuleGroupsValid(groups), true)
          assert.ok(
            rules.tryParseRequestRuleExpr(rules.buildRequestRuleExpr(groups))
          )
        }
        for (const value of [min - 1, max + 1, min + 0.5]) {
          assert.equal(
            rules.tryParseRequestRuleExpr(
              `(${timeFunc}("UTC") >= ${value} ? 2 : 1)`
            ),
            null
          )
        }
      }
    })
  })
}
