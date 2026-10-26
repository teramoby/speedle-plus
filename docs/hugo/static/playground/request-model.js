/** Form-only request conversion. No engine semantics are reproduced here. */
export function requestToForm(request = {}) {
  return {
    action: request.action || '', resource: request.resource || '', serviceName: request.serviceName || 'playground',
    principals: (request.subject?.principals || []).map(p => ({ type: p.type, name: p.name, idd: p.idd || '' })),
    attributes: (request.attributes || []).map(a => ({
      name:a.name, type:a.type, multiple:Array.isArray(a.value),
      values:(Array.isArray(a.value)?a.value:[a.value]).map(value => a.type==='datetime' && !Number.isNaN(Date.parse(value)) ? new Date(value).toISOString().slice(0,19) : String(value))
    }))
  };
}
export function serializeRequest(state) {
  const errors=[];
  const error=(path,message)=>errors.push({path,message});
  const action=String(state.action || '').trim(), resource=String(state.resource || '').trim();
  if(!action)error('action','Enter an action, such as read.');
  if(!resource)error('resource','Enter a resource, such as book.');
  const principals=(state.principals || []).map((p,i)=>{
    if(!['user','group','entity'].includes(p.type))error(`principals.${i}.type`,'Choose user, group or entity.');
    const name=String(p.name || '').trim(),idd=String(p.idd || '').trim();
    if(!name)error(`principals.${i}.name`,'Enter a principal name, or remove this principal.');
    return {type:p.type,name,...(idd?{idd}:{})};
  });
  if(principals.length>200)error('principals','A maximum of 200 principals is supported.');
  const names=new Set();
  const attributes=(state.attributes || []).map((a,i)=>{
    const name=String(a.name || '').trim(), type=a.type;
    if(!/^[a-zA-Z][a-zA-Z0-9_]{0,254}$/.test(name))error(`attributes.${i}.name`,'Start with a letter and use only letters, numbers or underscores (maximum 255 characters).');
    else if(name.startsWith('request_'))error(`attributes.${i}.name`,'Attributes starting with request_ are supplied by the engine and cannot be set here.');
    else if(names.has(name))error(`attributes.${i}.name`,'Duplicate attribute name. Use a different name.');
    names.add(name);
    if(!['string','numeric','bool','datetime'].includes(type))error(`attributes.${i}.type`,'Choose a valid attribute type.');
    const rawValues=Array.isArray(a.values)?a.values:[];
    if(!a.multiple && rawValues.length!==1)error(`attributes.${i}.values`,'A scalar attribute requires exactly one value.');
    const values=rawValues.map((raw,j)=>{
      const text=String(raw ?? ''); const path=`attributes.${i}.values.${j}`;
      if(type==='string')return text;
      if(type==='numeric'){
        if(!text.trim() || !Number.isFinite(Number(text))) {error(path,'Enter a valid number.');return null;}
        return Number(text);
      }
      if(type==='bool'){
        if(text!=='true' && text!=='false'){error(path,'Choose true or false.');return null;}
        return text==='true';
      }
      if(type==='datetime'){
        if(!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(?::\d{2}(?:\.\d{1,3})?)?$/.test(text)){error(path,'Choose a valid UTC date and time.');return null;}
        const date=new Date(`${text}Z`);
        if(Number.isNaN(date.getTime()) || date.toISOString().slice(0,16)!==text.slice(0,16)){error(path,'Choose a valid UTC date and time.');return null;}
        return date.toISOString();
      }
      return text;
    });
    return {name,type,value:a.multiple?values:values[0]};
  });
  if(errors.length)return {ok:false,errors};
  return {ok:true,request:{subject:{principals},action,resource,serviceName:String(state.serviceName || '').trim() || 'playground',...(attributes.length?{attributes}:{})}};
}
