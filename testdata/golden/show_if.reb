<reb-select name="work_type" label="Work type" options="Hot work,Cold work" required help="Pick one"></reb-select>
<reb-text name="fire_watch" label="Fire watch" show-if="work_type == 'Hot work'" required placeholder="Name"></reb-text>
<reb-checkbox name="extinguisher" label="Extinguisher" show-if="fire_watch" required></reb-checkbox>
<reb-number name="crew" label="Crew" min="1" max="12" step="1" default="2"></reb-number>
<reb-date name="start" label="Start" min="2026-01-01" max="2026-12-31" default="today"></reb-date>
<reb-text name="permit_code" label="Permit code" pattern="[A-Z]{2}-\d{3}"></reb-text>
<reb-textarea name="remarks" label="Remarks" show-if="not (crew == 0) and work_type != ''"></reb-textarea>
<p>{{.work_type}} {{.fire_watch}} {{.extinguisher}} {{.crew}} {{formatDate "02/01/2006" .start}} {{.permit_code}}</p>
<div>{{.remarks | safeHTML}}</div>
