import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of } from 'rxjs';
import { RefundRecord } from '../admin.models';
import { AdminService } from '../admin.service';
import { RefundsComponent } from './refunds.component';

describe('RefundsComponent', () => {
  let component: RefundsComponent;
  let fixture: ComponentFixture<RefundsComponent>;
  let adminServiceSpy: jasmine.SpyObj<AdminService>;

  const mockRefunds: RefundRecord[] = [
    {
      id: 'refund-001',
      order_id: 'order-001',
      amount: 2500.0,
      status: 'PENDING',
      reason: 'Item arrived damaged',
      created_at: '2026-11-08T10:00:00Z',
    },
    {
      id: 'refund-002',
      order_id: 'order-002',
      amount: 1400.0,
      status: 'PENDING',
      reason: 'Wrong size delivered',
      created_at: '2026-11-09T10:00:00Z',
    },
    {
      id: 'refund-003',
      order_id: 'order-003',
      amount: 500.0,
      status: 'APPROVED',
      reason: 'Late delivery compensation',
      created_at: '2026-11-07T10:00:00Z',
      reviewed_at: '2026-11-08T12:00:00Z',
    },
  ];

  beforeEach(async () => {
    adminServiceSpy = jasmine.createSpyObj<AdminService>('AdminService', [
      'listRefunds',
      'updateRefundStatus',
    ]);
    adminServiceSpy.listRefunds.and.returnValue(of(mockRefunds));
    adminServiceSpy.updateRefundStatus.and.returnValue(
      of({
        ...mockRefunds[0],
        status: 'REJECTED',
        reason: 'Return window expired',
      }),
    );

    await TestBed.configureTestingModule({
      imports: [RefundsComponent],
      providers: [{ provide: AdminService, useValue: adminServiceSpy }],
    }).compileComponents();

    fixture = TestBed.createComponent(RefundsComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should render pending refunds by default and compute pending summary', () => {
    const compiled = fixture.nativeElement as HTMLElement;
    const rows = compiled.querySelectorAll('tbody tr');
    expect(rows.length).toBe(2);
    expect(component.pendingSummary()).toEqual({ count: 2, totalAmount: 3900 });
    expect(compiled.textContent).toContain('LKR 3,900.00');
  });

  it('should display the empty state when a tab has no refunds', () => {
    component.selectTab('REJECTED');
    fixture.detectChanges();

    const compiled = fixture.nativeElement as HTMLElement;
    expect(compiled.textContent).toContain('No rejected refunds');
  });

  it('should open the reject dialog and require a reason before submitting', () => {
    component.openRejectDialog(mockRefunds[0]);
    fixture.detectChanges();

    const dialog = (fixture.nativeElement as HTMLElement).querySelector('[role="dialog"]');
    expect(dialog?.textContent).toContain('Reject refund refund-001?');
    expect(component.rejectForm.invalid).toBeTrue();

    component.rejectForm.controls.reason.setValue('Return window expired');
    expect(component.rejectForm.valid).toBeTrue();

    component.confirmReject();
    expect(adminServiceSpy.updateRefundStatus).toHaveBeenCalledWith('refund-001', {
      status: 'REJECTED',
      reason: 'Return window expired',
    });
  });
});
