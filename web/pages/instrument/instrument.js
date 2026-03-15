const $ = sel => document.querySelector(sel);
const API_BASE = (() => {
      try {
        const override = localStorage.getItem('gtvApiBase');
        if (override) return override.replace(/\/+$/, '');
      } catch {}
      return '';
    })();
    const apiFetch = (path, options) => fetch(`${API_BASE}${path}`, options);
    let lastTimeline = null;
    let activeWorkloadName = '';
    function setStatus(msg){ $('#status').textContent = msg || ''; }
    function setOut(text){ $('#out').textContent = text || ''; }
    function setWorkloadView(text){ $('#workloadView').value = text || ''; }

    function setActiveWorkload(name, content){
      activeWorkloadName = (name || '').trim().toLowerCase();
      if (activeWorkloadName) {
        $('#name').value = activeWorkloadName;
        LS.setStr('name', activeWorkloadName);
        updateLiveGraphLink();
      }
      if (content) {
        setWorkloadView(content);
      } else if (activeWorkloadName) {
        setWorkloadView(`// Workload '${activeWorkloadName}' loaded.`);
      } else {
        setWorkloadView('');
      }
    }

	    // Persist simple UI choices in localStorage so they survive reloads
	    const LS = {
	      getBool(key, def){ const v = localStorage.getItem('instr.'+key); if(v===null) return !!def; return v==='1'; },
	      setBool(key, val){ localStorage.setItem('instr.'+key, val ? '1' : '0'); },
	      getStr(key, def){ const v = localStorage.getItem('instr.'+key); return v===null ? (def||'') : v; },
	      setStr(key, val){ localStorage.setItem('instr.'+key, val || ''); },
	    };
	    const SYNC_VALIDATION_KEY = 'gtv.sync.validation';
	    function getSyncValidationFlag(){
	      try { return localStorage.getItem(SYNC_VALIDATION_KEY) === '1'; } catch { return false; }
	    }
	    function setSyncValidationFlag(on){
	      try { localStorage.setItem(SYNC_VALIDATION_KEY, on ? '1' : '0'); } catch {}
	    }
	    function syncValidationComboActive(){
	      return ($('#level')?.value === 'regions_logs')
	        && ($('#mode')?.value === 'debug')
	        && !!($('#blockRegions')?.checked);
	    }
	    function refreshSyncValidationFlagFromControls(){
	      setSyncValidationFlag(syncValidationComboActive());
	    }

    function loadSettings(){
      // Instrument settings
      $('#name').value = LS.getStr('name', 'MyWork');
      $('#guard').checked = LS.getBool('guard', true);
      $('#gorRegions').checked = LS.getBool('gorRegions', true);
      $('#level').value = LS.getStr('level', 'regions_logs');
      $('#mode').value = LS.getStr('mode', 'debug');
      $('#valueLogs').checked = LS.getBool('valueLogs', false);
      $('#blockRegions').checked = LS.getBool('blockRegions', true);
      $('#ioRegions').checked = LS.getBool('ioRegions', false);
      const ioJSON = $('#ioJSON'); if (ioJSON) ioJSON.checked = LS.getBool('ioJSON', false);
      const ioDB = $('#ioDB'); if (ioDB) ioDB.checked = LS.getBool('ioDB', false);
      const ioHTTP = $('#ioHTTP'); if (ioHTTP) ioHTTP.checked = LS.getBool('ioHTTP', false);
      const ioOS = $('#ioOS'); if (ioOS) ioOS.checked = LS.getBool('ioOS', false);
      // Run settings
      $('#synth').checked = LS.getBool('synth', true);
      $('#drop').checked = LS.getBool('drop', false);
      $('#bcMode').value = LS.getStr('bcMode', '');
      const timeoutMs = LS.getStr('timeoutMs', '1000');
      if ($('#timeoutMs')) $('#timeoutMs').value = timeoutMs;
      if ($('#buildCache')) $('#buildCache').checked = LS.getBool('buildCache', true);
      updateTimeoutWarn();
    }

    function bindPersistence(){
      const bind = (id, kind) => {
        const el = $(id);
        if (!el) return;
        el.addEventListener('change', () => {
          if (kind==='bool') LS.setBool(id.slice(1), el.checked);
          else LS.setStr(id.slice(1), el.value);
          if (id==='#name') updateLiveGraphLink();
        });
      };
      bind('#name','str');
      bind('#guard','bool');
      bind('#gorRegions','bool');
      bind('#level','str');
      bind('#mode','str');
      bind('#valueLogs','bool');
      bind('#blockRegions','bool');
      bind('#ioRegions','bool');
      if ($('#ioJSON')) bind('#ioJSON','bool');
      if ($('#ioDB')) bind('#ioDB','bool');
      if ($('#ioHTTP')) bind('#ioHTTP','bool');
      if ($('#ioOS')) bind('#ioOS','bool');
      bind('#synth','bool');
      bind('#drop','bool');
      bind('#bcMode','str');
    }

    function updateLiveGraphLink(){
      const name = ($('#name').value || '').trim().toLowerCase();
      const synth = $('#synth').checked ? '1' : '0';
      const mode = ($('#mode').value || 'debug').trim().toLowerCase();
      if (!name){ $('#liveGraphLink').style.display='none'; return; }
      const params = new URLSearchParams({ name, synth, mode, return: '../instrument/instrument.html' });
      const tms = parseInt(($('#timeoutMs')?.value || '0'), 10);
      if (!Number.isNaN(tms) && tms > 0) params.set('timeout_ms', String(tms));
      $('#liveGraphLink').href = '../graph-live/graph-live.html?' + params.toString();
      $('#liveGraphLink').style.display='inline-block';
    }

    $('#btnLoadCode').onclick = () => $('#fileGoCode').click();
    $('#fileGoCode').onchange = async (e) => {
      const f = e.target.files[0];
      if (!f) return;
      if (!/\.go$/i.test(f.name)) {
        setStatus('Only .go files can be loaded.');
        return;
      }
      const text = await f.text();
      $('#code').value = text;
      setStatus(`Loaded Go code from ${f.name}`);
    };

    async function fetchServerWorkloads(){
      const res = await apiFetch('/workloads');
      let payload = null;
      try { payload = await res.json(); } catch {}
      if (!res.ok) {
        const msg = payload && payload.error ? payload.error : String(res.status);
        throw new Error(msg);
      }
      const items = payload && Array.isArray(payload.workloads) ? payload.workloads : [];
      return items.map((x) => String(x || '').trim().toLowerCase()).filter(Boolean);
    }

    async function loadWorkloadByName(name){
      const wlRes = await apiFetch(`/workload?name=${encodeURIComponent(name)}`);
      if (!wlRes.ok) {
        const j = await wlRes.json().catch(() => ({}));
        throw new Error((j && j.error) || `HTTP ${wlRes.status}`);
      }
      const wlText = await wlRes.text();
      setActiveWorkload(name, wlText);
      setStatus(`Loaded workload '${name}' from internal/workload.`);
    }

    $('#btnLoadWorkload').onclick = async () => {
      try {
        const workloads = await fetchServerWorkloads();
        if (!workloads.length) {
          setStatus('No generated workloads found in internal/workload.');
          return;
        }
        const current = (activeWorkloadName || ($('#name').value || '')).trim().toLowerCase();
        const suggested = workloads.includes(current) ? current : workloads[0];
        const preview = workloads.slice(0, 20).join(', ');
        const suffix = workloads.length > 20 ? ` ... (+${workloads.length - 20} more)` : '';
        const picked = window.prompt(
          `Load from internal/workload.\nEnter workload name.\nAvailable: ${preview}${suffix}`,
          suggested,
        );
        if (picked === null) return;
        const name = picked.trim().toLowerCase();
        if (!name) {
          setStatus('No workload selected.');
          return;
        }
        if (!workloads.includes(name)) {
          setStatus(`Workload '${name}' not found in internal/workload.`);
          return;
        }
        await loadWorkloadByName(name);
      } catch (e) {
        setStatus(`Load workload error: ${e.message}`);
      }
    };

	    $('#btnInstrument').onclick = async () => {
	      setStatus(''); setOut('');
	      const code = $('#code').value;
	      const name = ($('#name').value || 'MyWork').trim();
      if (!code.trim()) { setStatus('Paste code first.'); return; }
	      // persist name on instrument
	      LS.setStr('name', name);
	      try{
	        refreshSyncValidationFlagFromControls();
	        const res = await apiFetch('/instrument', { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ code, workload_name: name, guard_labels: $('#guard').checked, goroutine_regions: $('#gorRegions').checked, level: $('#level').value, sync_validation: getSyncValidationFlag(), block_regions: $('#blockRegions').checked, io_regions: $('#ioRegions').checked, io_json: $('#ioJSON').checked, io_db: $('#ioDB').checked, io_http: ($('#ioHTTP')?$('#ioHTTP').checked:false), io_os: ($('#ioOS')?$('#ioOS').checked:false), value_logs: $('#valueLogs').checked }) });
	        const j = await res.json();
	        if (!res.ok) throw new Error(j && j.error || res.status);
	        const syncNote = j && j.sync_validation_applied ? ' (sync validation preset enforced)' : '';
	        setStatus(`Workload '${j.workload}' created → ${j.file}${syncNote}`);
	        updateLiveGraphLink();
        try {
          const wlRes = await apiFetch(`/workload?name=${encodeURIComponent(j.workload)}`);
          if (wlRes.ok) {
            const wlText = await wlRes.text();
            setActiveWorkload(j.workload, wlText);
          } else {
            setActiveWorkload(j.workload, '');
          }
        } catch {
          setActiveWorkload(j.workload, '');
        }
      }catch(e){ setStatus('Instrument error: '+ e.message); }
    };

    $('#btnRun').onclick = async () => {
      setOut('Running...'); lastTimeline = null; $('#dlHint').textContent='';
      const name = ($('#name').value || 'MyWork').trim().toLowerCase();
      const synth = $('#synth').checked ? '1' : '0';
      const drop = $('#drop').checked ? '1' : '0';
      const mode = ($('#mode').value || 'debug').trim().toLowerCase();
      const bcMode = ($('#bcMode').value || '').trim();
      // persist run settings
      LS.setStr('name', ($('#name').value||'MyWork').trim());
      LS.setBool('synth', $('#synth').checked);
      LS.setBool('drop', $('#drop').checked);
      LS.setStr('bcMode', bcMode);
      LS.setStr('mode', mode);
      if ($('#timeoutMs')) LS.setStr('timeoutMs', $('#timeoutMs').value);
      if ($('#buildCache')) LS.setBool('buildCache', $('#buildCache').checked);
      try{
        const q = new URLSearchParams({ name, synth, drop_block_no_ch: drop, mode });
        if (bcMode) q.set('bc_mode', bcMode);
        const tms = parseInt(($('#timeoutMs')?.value || '1000'), 10);
        if (!Number.isNaN(tms) && tms > 0) q.set('timeout_ms', String(tms));
        if ($('#buildCache') && $('#buildCache').checked) {
          q.set('build_cache', '1');
        } else {
          q.set('build_cache', '0');
        }
        const res = await apiFetch(`/run?${q.toString()}`);
        const text = await res.text();
        let payload = null;
        try { payload = JSON.parse(text); } catch {}
        if (!res.ok) {
          const msg = payload && payload.error ? payload.error : (text || String(res.status));
          throw new Error(msg);
        }
        lastTimeline = Array.isArray(payload) ? payload : (payload && payload.events) || [];
        setOut(JSON.stringify(lastTimeline, null, 2));
        $('#dlHint').textContent = ` ${lastTimeline.length} events`;
        updateLiveGraphLink();
      }catch(e){ setOut('Run error: '+ e.message); }
    };

    $('#btnDownload').onclick = () => {
      if (!lastTimeline) { setOut('No timeline to download'); return; }
      const b = new Blob([JSON.stringify(lastTimeline)], {type:'application/json'});
      const url = URL.createObjectURL(b);
      const a = document.createElement('a'); a.href = url; a.download = 'trace.json'; a.click(); setTimeout(()=>URL.revokeObjectURL(url), 1000);
    };

    $('#btnClearCache')?.addEventListener('click', async () => {
      try{
        const res = await fetch('/clear-build-cache', { method: 'POST' });
        const j = await res.json();
        if (!res.ok) throw new Error(j && j.error || res.status);
        setStatus('Build cache cleared.');
      }catch(e){
        setStatus('Clear cache error: ' + e.message);
      }
    });

	    $('#btnSyncInstrPreset')?.addEventListener('click', () => {
	      $('#level').value = 'regions_logs';
	      $('#mode').value = 'debug';
	      $('#blockRegions').checked = true;
	      $('#gorRegions').checked = true;
	      setSyncValidationFlag(true);
	      LS.setStr('level', 'regions_logs');
	      LS.setStr('mode', 'debug');
	      LS.setBool('blockRegions', true);
	      LS.setBool('gorRegions', true);
      updateLiveGraphLink();
      setStatus('Applied sync capture preset: regions_logs + debug mode + block regions.');
    });

	    // Initialize UI
	    loadSettings();
	    ['#level', '#mode', '#blockRegions'].forEach((sel) => {
	      const el = $(sel);
	      if (!el) return;
	      el.addEventListener('change', () => refreshSyncValidationFlagFromControls());
	    });
	    refreshSyncValidationFlagFromControls();
	    bindPersistence();
	    updateLiveGraphLink();

    function updateTimeoutWarn(){
      const el = $('#timeoutWarn');
      if (!el) return;
      const tms = parseInt(($('#timeoutMs')?.value || '0'), 10);
      if (!Number.isNaN(tms) && tms > 0 && tms < 500) {
        el.textContent = 'Low timeout may fail build';
        el.style.display = 'inline';
      } else {
        el.textContent = '';
        el.style.display = 'none';
      }
    }
    $('#timeoutMs')?.addEventListener('input', () => {
      updateTimeoutWarn();
      updateLiveGraphLink();
    });
