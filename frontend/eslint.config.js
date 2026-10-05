// ESLint flat config. Two rule sets, both of which flag defects rather than
// taste: typescript-eslint's recommended set, and the react-hooks rules — which
// are what catch a missing or unstable dependency array, the exact class of bug
// that made the history window re-query on every parent render.
//
// The plugin's own `recommended-latest` preset is still written in the legacy
// `plugins: [...]` form, which ESLint 10 rejects, so the two rules are named
// here instead.
import tseslint from 'typescript-eslint';
import reactHooks from 'eslint-plugin-react-hooks';

export default tseslint.config(
  // Generated and vendor trees are not ours to lint.
  { ignores: ['dist/**', 'wailsjs/**', 'node_modules/**', '.npm-cache/**', 'eslint.config.js'] },
  ...tseslint.configs.recommended,
  {
    files: ['src/**/*.{ts,tsx}'],
    plugins: { 'react-hooks': reactHooks },
    rules: {
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'warn',
      // An unused argument is normal in a callback signature; an unused local is
      // already an error through tsc's noUnusedLocals.
      '@typescript-eslint/no-unused-vars': ['error', { args: 'none', varsIgnorePattern: '^_' }],
    },
  },
);
