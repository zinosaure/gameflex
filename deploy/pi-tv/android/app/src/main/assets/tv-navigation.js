(function(){
 if(window.GameFlexTv)return;
 const control=()=>document.activeElement;
 const surface=()=>document.querySelector('[aria-modal="true"]')||document.querySelector('.ps-console')||document.body;
 const first=()=>surface().querySelector('.ps-console-tile[aria-pressed="true"]')||surface().querySelector('button:not(:disabled),a[href],input,select');
 window.GameFlexTv={
  returnToLibrary(){if(history.length>1)history.back();else location.replace('/?tv=1');},
  move(direction){let el=control();if(!el||el===document.body){first()?.focus();return;}const key={left:'ArrowLeft',right:'ArrowRight',up:'ArrowUp',down:'ArrowDown'}[direction];if(key)el.dispatchEvent(new KeyboardEvent('keydown',{key,code:key,bubbles:true,cancelable:true}));},
  ok(){const el=control();if(el?.matches('input,textarea,select')){el.focus();return 'input';}el?.click();return 'button';},
  back(){const dialog=document.querySelector('[aria-modal="true"]');if(dialog){const close=dialog.querySelector('button[aria-label^="Fermer"],button[aria-label="Retour"]');if(close)close.click();else (control()||dialog).dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true,cancelable:true}));return;}if(location.pathname==='/'&&!location.search.includes('section='))location.href='gameflex-tv://settings';else if(location.pathname!=='/')location.href='/?tv=1';else history.back();}
 };
 if(control()===document.body)first()?.focus();
})();
