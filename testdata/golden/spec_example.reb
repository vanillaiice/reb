<!-- REBAR COMPILER v1.0 SPECIFICATION STANDARD -->
<reb-tailwind></reb-tailwind>

<style>
  body {
    font-family: Arial, sans-serif;
    color: #1a202c;
    line-height: 1.6;
  }
  .header-card {
    border-left: 6px solid #d97706;
    background-color: #fef3c7;
    padding: 16px;
    margin-bottom: 20px;
  }
  .meta-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
    margin-bottom: 30px;
  }
</style>

<div class="header-card">
  <h2>High Risk Work Authorization Certificate</h2>
</div>

<div class="meta-grid">
  <div>
    <strong>Permit Reference:</strong>
    <reb-number name="permit_number" label="Authorized Permit Number" class="text-slate-800 font-semibold"></reb-number>
  </div>
  <div>
    <strong>Clearance Category:</strong>
    <reb-select name="clearance_type" label="Operational Clearance Category" options="Hot Work, Confined Space Entry, Electrical Isolation"></reb-select>
  </div>
</div>

<div class="status-box">
  <p><strong>PPE Controls Verified:</strong> 
    <reb-text name="ppe_verified" label="Protective Gears Inspected"></reb-text>
  </p>
  
  <h3 class="mt-4 font-bold">Additional Notes</h3>
  <reb-textarea name="notes" label="Site Supervisor Remarks"></reb-textarea>
</div>

<reb-footer>
  <div style="width: 100%; text-align: center; font-size: 10px; color: #718096; padding-top: 10px; border-top: 1px solid #cbd5e1; margin-top: 20px;">
    Rebar Automated Document System — Page <span class="pageNumber"></span> of <span class="totalPages"></span>
  </div>
</reb-footer>
