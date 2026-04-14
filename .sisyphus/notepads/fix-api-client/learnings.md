## Patterns and Conventions
- API client uses both XSRF token header and hglogin cookie for authentication.
- All request methods must call setHeaders and setCookies.

## Successful Approaches
- Rewriting the entire file was more efficient than trying to patch multiple syntax errors and duplications.
