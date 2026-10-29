/** Parses principals written as policies write them: [idd=<IDD>:]<type>:<name>. */
const PRINCIPAL = /^(?:idd=([^:\s]+):)?(user|group|entity):(\S.*)$/;

export function parsePrincipals(text) {
  const principals = [];
  const errors = [];
  String(text ?? '').split('\n').map((s) => s.trim()).filter(Boolean).forEach((line, i) => {
    const m = line.match(PRINCIPAL);
    if (!m) { errors.push({ path: `principals.${i}`, message: 'Write subject principals as type:name with type user, group or entity, such as user:alice.' }); return; }
    principals.push(m[1] ? { type: m[2], name: m[3], idd: m[1] } : { type: m[2], name: m[3] });
  });
  return { principals, errors };
}
