import { HttpInterceptorFn } from '@angular/common/http';
import { environment } from '../../../environments/environment';

export const authInterceptor: HttpInterceptorFn = (req, next) => {
  if (!req.url.startsWith(environment.apiBaseUrl)) {
    return next(req);
  }

  const token = (localStorage.getItem('access_token') ?? '').trim();
  const storedRole = (localStorage.getItem('user_role') ?? '').trim();
  const storedId = (localStorage.getItem('user_id') ?? '').trim();
  const storedName = (localStorage.getItem('user_name') ?? '').trim();

  const actorRole = storedRole || (!environment.production ? 'admin' : '');
  const actorId = storedId || (!environment.production ? 'admin-001' : '');
  const actorName = storedName || (!environment.production ? 'System Admin' : '');

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
