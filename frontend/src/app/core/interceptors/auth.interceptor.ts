import { HttpInterceptorFn } from '@angular/common/http';
import { environment } from '../../../environments/environment';
import { resolveDevActorForUrl } from '../auth/dev-actors';

export const authInterceptor: HttpInterceptorFn = (req, next) => {
  if (!req.url.startsWith(environment.apiBaseUrl)) {
    return next(req);
  }

  const token = (localStorage.getItem('access_token') ?? '').trim();
  const storedRole = (localStorage.getItem('user_role') ?? '').trim();
  const storedId = (localStorage.getItem('user_id') ?? '').trim();
  const storedName = (localStorage.getItem('user_name') ?? '').trim();

  // DEV ONLY - replace with real auth once Member 01's authentication module is merged.
  const fallbackActor = !environment.production ? resolveDevActorForUrl(req.url) : null;

  const actorRole = storedRole || (fallbackActor ? fallbackActor.role : '');
  const actorId = storedId || (fallbackActor ? fallbackActor.id : '');
  const actorName = storedName || (fallbackActor ? fallbackActor.name : '');

  let headers = req.headers;
  if (token) {
    headers = headers.set('Authorization', `Bearer ${token}`);
  }
  headers = headers
    .set('X-Actor-Role', actorRole)
    .set('X-Actor-ID', actorId)
    .set('X-Actor-Name', actorName);

  return next(req.clone({ headers }));
};
