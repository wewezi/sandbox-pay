import { NestFactory } from '@nestjs/core';
import { ValidationPipe, VersioningType } from '@nestjs/common';
import { SwaggerModule, DocumentBuilder } from '@nestjs/swagger';
import helmet from 'helmet';
import cookieParser from 'cookie-parser';
import { AppModule } from './app.module';
import { AllExceptionsFilter } from './common/filters/all-exceptions.filter';
import { TransformInterceptor } from './common/interceptors/transform.interceptor';
import { corsOriginDelegate } from './common/util/cors-origin';
import { validateEnv } from './common/util/validate-env';
import { createWinstonLogger } from './common/logging/winston.config';

async function bootstrap() {
  validateEnv();

  const app = await NestFactory.create(AppModule, {
    abortOnError: false,
    logger: createWinstonLogger(),
  });

  // Graceful shutdown — wait for in-flight requests to complete before exiting
  app.enableShutdownHooks();

  // Cookie parser — 读取本应用 host-only JWT 会话 cookie
  app.use(cookieParser());

  // API versioning — URI-based with v1 as default.
  // Existing routes (no explicit version) remain at /api/xxx for backward
  // compatibility. Future v2 controllers use @Controller({ version: '2' })
  // and will be served at /v2/api/xxx.
  app.enableVersioning({
    type: VersioningType.URI,
    defaultVersion: '1',
    prefix: 'v',
  });

  // Global pipes
  app.useGlobalPipes(
    new ValidationPipe({
      whitelist: true,
      transform: true,
      forbidNonWhitelisted: true,
    }),
  );

  // Global interceptors
  app.useGlobalInterceptors(new TransformInterceptor());

  // Global filters
  app.useGlobalFilters(new AllExceptionsFilter());

  // CORS — only allow localhost (any port) and *.we29.cn subdomains
  app.enableCors({
    origin: corsOriginDelegate,
    credentials: true,
    methods: ['GET', 'POST', 'PUT', 'DELETE', 'PATCH', 'OPTIONS'],
    allowedHeaders: ['Content-Type', 'Authorization', 'X-Request-Id'],
  });

  // Security headers (X-Content-Type-Options, X-Frame-Options, HSTS, CSP, etc.)
  app.use(helmet());

  // Raw health endpoint — bypasses NestJS URI versioning for Railway healthchecks
  const httpAdapter = app.getHttpAdapter();
  httpAdapter.get('/health', (_req: any, res: any) => {
    res.json({ status: 'ok', redis: 'connected' });
  });

  // Swagger is opt-in. Keep interface details off public deployments by default;
  // the authenticated client console contains the merchant integration guide.
  if (process.env.ENABLE_API_DOCS === 'true') {
    const config = new DocumentBuilder()
      .setTitle('Sandbox Pay API')
      .setDescription(
        'Sandbox Pay is an aggregated payment gateway for development and testing.\n\n' +
          '**Authentication**\n' +
          '- **Admin endpoints** — local JWT session cookie or Bearer token.\n' +
          '- **Merchant gateway** — HMAC-SHA256 request signing using the assigned merchant secret.\n\n' +
          '**Security**\n' +
          'Do not enable this Swagger UI on a publicly reachable production deployment.',
      )
      .setVersion('1.0')
      .addBearerAuth()
      .build();
    const document = SwaggerModule.createDocument(app, config);
    SwaggerModule.setup('v1/api/docs', app, document);
  }

  await app.listen(process.env.PORT || 3000);
}
void bootstrap();
