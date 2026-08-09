export function normalizeLogSettingsValue(defaultValue, optionValue) {
  if (typeof defaultValue === 'boolean') {
    return (
      optionValue === true || optionValue === 'true' || optionValue === '1'
    );
  }
  return optionValue;
}

export function buildLogSettingsInputs(defaultInputs, options) {
  const nextInputs = { ...defaultInputs };
  for (let key in options || {}) {
    if (Object.prototype.hasOwnProperty.call(defaultInputs, key)) {
      nextInputs[key] = normalizeLogSettingsValue(
        defaultInputs[key],
        options[key],
      );
    }
  }
  return nextInputs;
}
