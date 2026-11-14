import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of } from 'rxjs';
import { SellerApplication } from '../admin.models';
import { AdminService } from '../admin.service';
import { SellerApprovalsComponent } from './seller-approvals.component';

describe('SellerApprovalsComponent', () => {
  let component: SellerApprovalsComponent;
  let fixture: ComponentFixture<SellerApprovalsComponent>;
  let adminServiceSpy: jasmine.SpyObj<AdminService>;

  const mockApplications: SellerApplication[] = [
    {
      id: 'seller-002',
      user_id: 'seller-u2',
      user_name: 'Amali Weaves',
      user_email: 'amali@craftbridge.test',
      store_name: 'Amali Handlooms',
      status: 'PENDING',
      notes: 'Traditional cotton weaves',
      submitted_at: '2026-11-05T10:00:00Z',
    },
    {
      id: 'seller-001',
      user_id: 'seller-u1',
      user_name: 'Sunil Craftsman',
      user_email: 'sunil@craftbridge.test',
      store_name: 'Sunil Wood Crafts',
      status: 'APPROVED',
      notes: 'Verified artisan registration',
      submitted_at: '2026-11-01T10:00:00Z',
      reviewed_at: '2026-11-02T12:00:00Z',
    },
  ];

  beforeEach(async () => {
    adminServiceSpy = jasmine.createSpyObj<AdminService>('AdminService', [
      'listSellerApplications',
      'updateSellerStatus',
    ]);
    adminServiceSpy.listSellerApplications.and.returnValue(of(mockApplications));
    adminServiceSpy.updateSellerStatus.and.returnValue(
      of({
        ...mockApplications[0],
        status: 'REJECTED',
        notes: 'Incomplete portfolio photos',
      }),
    );

    await TestBed.configureTestingModule({
      imports: [SellerApprovalsComponent],
      providers: [{ provide: AdminService, useValue: adminServiceSpy }],
    }).compileComponents();

    fixture = TestBed.createComponent(SellerApprovalsComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should render pending applications by default and compute tab counts', () => {
    const compiled = fixture.nativeElement as HTMLElement;
    const rows = compiled.querySelectorAll('tbody tr');
    expect(rows.length).toBe(1);
    expect(rows[0].textContent).toContain('Amali Handlooms');
    expect(rows[0].textContent).toContain('Amali Weaves');
    expect(rows[0].textContent).toContain('amali@craftbridge.test');
    expect(component.counts()).toEqual({
      PENDING: 1,
      APPROVED: 1,
      REJECTED: 0,
      ALL: 2,
    });
  });

  it('should display the empty state when the selected tab has no applications', () => {
    component.selectTab('REJECTED');
    fixture.detectChanges();

    const compiled = fixture.nativeElement as HTMLElement;
    expect(compiled.textContent).toContain('No rejected applications');
  });

  it('should open the reject dialog and require notes before submitting', () => {
    component.openRejectDialog(mockApplications[0]);
    fixture.detectChanges();

    const dialog = (fixture.nativeElement as HTMLElement).querySelector('[role="dialog"]');
    expect(dialog?.textContent).toContain('Reject Amali Handlooms (Amali Weaves)?');
    expect(component.rejectForm.invalid).toBeTrue();

    component.rejectForm.controls.notes.setValue('Incomplete portfolio photos');
    expect(component.rejectForm.valid).toBeTrue();

    component.confirmReject();
    expect(adminServiceSpy.updateSellerStatus).toHaveBeenCalledWith('seller-002', {
      status: 'REJECTED',
      notes: 'Incomplete portfolio photos',
    });
  });
});
