import { TokenPanel } from './TokenPanel';
import { AuthSection } from './AuthSection';
import { Section } from './Section';
import { useSettingsSection } from './useSettingsSection';

/** AuthSettingsSection is the Auth tab's route content: the Section Card
 * chrome (title, Save, saved/error) wrapping the existing field-only
 * AuthSection plus the API tokens panel, unchanged from the single-page
 * Settings layout. */
export function AuthSettingsSection() {
  const { auth, setAuth, locked, saving, error, saved, saveAuth } = useSettingsSection('auth');

  return (
    <Section
      id="auth-heading" title="Auth" saving={saving}
      error={error} saved={saved}
      onSave={saveAuth}
    >
      <AuthSection value={auth} locked={locked} onChange={setAuth} />

      <div className="grid gap-2 border-t border-line pt-4">
        <h3 className="text-sm font-semibold text-fg">API tokens</h3>
        <p className="text-sm text-faint">
          Send a token as <code>Authorization: Bearer &lt;token&gt;</code>. The live-events
          stream also accepts <code>?token=</code>.
        </p>
        <TokenPanel />
      </div>
    </Section>
  );
}
