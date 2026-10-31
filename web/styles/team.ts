import { css } from 'lit';

export const teamStyles = css`
  .team-head, .modal-header, .member-chip { display: flex; align-items: center; justify-content: space-between; gap: var(--s3); }
  .team-columns { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: var(--s5); align-items: start; }
  .account-meta { display: flex; gap: var(--s3); align-items: center; flex-wrap: wrap; }
  .team-form { display: grid; grid-template-columns: minmax(140px, 1fr) minmax(160px, 1fr) minmax(160px, .8fr) auto; gap: var(--s3); align-items: end; }
  .user-actions { display: flex; gap: var(--s2); flex-wrap: wrap; min-width: 280px; }
  .user-actions button { padding: 6px 10px; font-size: 12px; }
  .team-table { min-width: 1000px; }
  .team-table td { vertical-align: middle; white-space: nowrap; }
  .team-table .project-names { min-width: 180px; max-width: 320px; white-space: normal; }
  .project-name { display:block; line-height:1.7; }
  .session-table td:first-child { min-width:240px; }
  .session-device { max-width: 360px; white-space: normal; overflow-wrap: anywhere; font-size: 12px; line-height:1.7; }
  .session-meta { display:flex; align-items:center; flex-wrap:wrap; gap:var(--s2); margin:var(--s1) 0; }
  .session-meta .mono { color:var(--muted); }
  .session-current { flex:none; }
  .session-created { line-height:1.65; }
  .password-form { max-width: 460px; }
  .member-list { display: flex; flex-wrap: wrap; gap: var(--s2); }
  .member-chip { max-width:100%; padding: var(--s2) var(--s3); border: 1px solid var(--line); border-radius: var(--r-sm); background: var(--panel-2); font-size: 13px; line-height:1.6; overflow-wrap:anywhere; }
  .member-chip button { flex:none; padding: 0; width: 24px; height: 24px; border: 0; background: none; color:var(--muted); }
  .member-add { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: var(--s2); align-items: end; }
  dialog.team-modal { max-width: 520px; }
  .modal-close { width: 34px; height: 34px; padding: 0; font-size: 20px; }
  .action-note { color: var(--muted); font-size: 12px; line-height:1.7; }
  @media (max-width: 1000px) { .team-form { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
  @media (max-width: 720px) { .team-columns { grid-template-columns: minmax(0, 1fr); gap:var(--s4); } .team-head { align-items: flex-start; } .team-head > button { flex:none; } }
  @media (max-width: 440px) { .team-form { grid-template-columns: minmax(0, 1fr); } }
`;
