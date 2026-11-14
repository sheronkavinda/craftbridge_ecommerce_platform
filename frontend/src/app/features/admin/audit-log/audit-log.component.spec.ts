import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of } from 'rxjs';
import { AuditLog } from '../admin.models';
import { AdminService } from '../admin.service';
import { AuditLogComponent } from './audit-log.component';

describe('AuditLogComponent', () => {
  let component: AuditLogComponent;
  let fixture: ComponentFixture<AuditLogComponent>;
  let mockAdminService: {
    listAuditLogs: jasmine.Spy;
  };

  const sampleLogs: AuditLog[] = [
    {
      id: 101,
      actor_id: 'admin-001',
      actor_name: 'System Admin',
      action: 'USER_STATUS_UPDATE',
      target: 'user:buyer-003',
      outcome: 'SUCCESS',
      request_id: 'req-abc-001',
      details: 'status set to SUSPENDED | reason: Policy violation',
      created_at: '2026-11-13T12:00:00Z',
    },
    {
      id: 100,
      actor_id: 'admin-001',
      actor_name: 'System Admin',
      action: 'CATEGORY_CREATE',
      target: 'category:cat-01',
      outcome: 'FAILURE',
      request_id: 'req-abc-002',
      details: 'category slug already exists',
      created_at: '2026-11-13T11:30:00Z',
    },
  ];

  beforeEach(async () => {
    mockAdminService = {
      listAuditLogs: jasmine.createSpy('listAuditLogs').and.returnValue(of(sampleLogs)),
    };

    await TestBed.configureTestingModule({
      imports: [AuditLogComponent],
      providers: [{ provide: AdminService, useValue: mockAdminService }],
    }).compileComponents();

    fixture = TestBed.createComponent(AuditLogComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('renders audit log entries from the mocked service', () => {
    expect(mockAdminService.listAuditLogs).toHaveBeenCalledWith('', '', '', 25, 0);
    expect(component.filteredLogs().length).toBe(2);
    const compiled = fixture.nativeElement as HTMLElement;
    expect(compiled.textContent).toContain('User status update');
    expect(compiled.textContent).toContain('user:buyer-003');
    expect(compiled.textContent).toContain('req-abc-001');
  });

  it('renders the empty state when no audit entries match', () => {
    component.onSearchChange('non-existent-search-term-xyz');
    fixture.detectChanges();

    expect(component.filteredLogs().length).toBe(0);
    const compiled = fixture.nativeElement as HTMLElement;
    expect(compiled.textContent).toContain('No audit entries match');
  });

  it('disables Previous pagination on the first page and disables Next when fewer than 25 entries are returned', () => {
    expect(component.currentPage()).toBe(1);
    expect(component.canGoPrevious()).toBeFalse();
    expect(component.canGoNext()).toBeFalse();
  });
});
