import { CallHandler, ExecutionContext, Injectable, NestInterceptor } from '@nestjs/common';
import { Observable } from 'rxjs';
import { map } from 'rxjs/operators';

export interface ApiResponse<T> {
  data: T;
  code: number;
  msg: string;
  message?: string;
}

@Injectable()
export class TransformInterceptor<T> implements NestInterceptor<T, ApiResponse<T>> {
  intercept(context: ExecutionContext, next: CallHandler): Observable<ApiResponse<T>> {
    return next.handle().pipe(
      map(data => {
        if (data && typeof data === 'object') {
          if (data.type === 'form' || data.type === 'url' || data.stream) {
            return { data, code: 200, msg: 'success' };
          }
          if ('code' in data && ('msg' in data || 'message' in data)) {
            return data;
          }
        }
        let msg = 'success';
        if (data && typeof data === 'object' && 'message' in data && typeof data.message === 'string') {
          msg = data.message;
        } else if (data && typeof data === 'object' && 'msg' in data && typeof data.msg === 'string') {
          msg = data.msg;
        }
        return {
          code: 200,
          msg,
          message: msg,
          data: data !== undefined ? data : null,
        };
      }),
    );
  }
}
